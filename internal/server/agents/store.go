package agents

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type fileStore struct {
	stateDir string
}

func newFileStore(stateDir string) *fileStore {
	return &fileStore{stateDir: stateDir}
}

func (s *fileStore) Load() ([]Config, error) {
	entries, err := os.ReadDir(s.agentsDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Config{}, nil
		}
		return nil, fmt.Errorf("read agents directory: %w", err)
	}

	agents := make([]Config, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		id := entry.Name()
		path, err := s.agentPath(id)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read agent config %q: %w", path, err)
		}

		var agent Config
		if err := json.Unmarshal(data, &agent); err != nil {
			return nil, fmt.Errorf("decode agent config %q: %w", path, err)
		}
		if agent.ID != id {
			return nil, fmt.Errorf("agent config %q has id %q, want %q", path, agent.ID, id)
		}
		if err := s.ensureDirectories(id); err != nil {
			return nil, err
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return nil, fmt.Errorf("secure agent config %q: %w", path, err)
		}
		agents = append(agents, agent)
	}

	return agents, nil
}

func (s *fileStore) Save(agent Config) error {
	if err := s.ensureDirectories(agent.ID); err != nil {
		return err
	}

	data, err := json.MarshalIndent(agent, "", "  ")
	if err != nil {
		return fmt.Errorf("encode agent config: %w", err)
	}
	data = append(data, '\n')

	path, err := s.agentPath(agent.ID)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".agent-*.json")
	if err != nil {
		return fmt.Errorf("create temporary agent config: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("secure temporary agent config: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary agent config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary agent config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace agent config: %w", err)
	}

	return nil
}

func (s *fileStore) ConfigDir(id string) (string, error) {
	if err := s.ensureDirectories(id); err != nil {
		return "", err
	}
	return filepath.Join(s.agentsDir(), id, "config"), nil
}

func (s *fileStore) ensureDirectories(id string) error {
	agentDir, err := s.agentDir(id)
	if err != nil {
		return err
	}
	configDir := filepath.Join(agentDir, "config")
	for _, dir := range []string{s.agentsDir(), agentDir, configDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create agent directory %q: %w", dir, err)
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("secure agent directory %q: %w", dir, err)
		}
	}
	return nil
}

func (s *fileStore) agentsDir() string {
	return filepath.Join(s.stateDir, "agents")
}

func (s *fileStore) agentDir(id string) (string, error) {
	if id == "" || filepath.Base(id) != id || strings.ContainsAny(id, `/\\`) {
		return "", fmt.Errorf("invalid agent id %q", id)
	}
	return filepath.Join(s.agentsDir(), id), nil
}

func (s *fileStore) agentPath(id string) (string, error) {
	dir, err := s.agentDir(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "agent.json"), nil
}
