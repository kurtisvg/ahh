package projects

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

func (s *fileStore) Load() ([]projectDefinition, error) {
	entries, err := os.ReadDir(s.projectsDir())
	if errors.Is(err, os.ErrNotExist) {
		return []projectDefinition{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read projects directory: %w", err)
	}

	definitions := make([]projectDefinition, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path, err := s.definitionPath(entry.Name())
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read project definition %q: %w", path, err)
		}
		var project projectDefinition
		decoder := json.NewDecoder(strings.NewReader(string(data)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&project); err != nil {
			return nil, fmt.Errorf("decode project definition %q: %w", path, err)
		}
		if project.ID != entry.Name() {
			return nil, fmt.Errorf("project definition %q has id %q, want %q", path, project.ID, entry.Name())
		}
		if project.ID != project.Name {
			return nil, fmt.Errorf("project definition %q has differing id and name", path)
		}
		if _, err := sourceFor(project.Source); err != nil {
			return nil, fmt.Errorf("validate project definition %q: %w", path, err)
		}
		if err := validateName(project.Name); err != nil {
			return nil, fmt.Errorf("project definition %q has invalid name: %w", path, err)
		}
		if err := validateDefaultBranch(project.DefaultBranch); err != nil {
			return nil, fmt.Errorf("validate project definition %q: %w", path, err)
		}
		if err := s.ensureProjectDir(project.ID); err != nil {
			return nil, err
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return nil, fmt.Errorf("secure project definition %q: %w", path, err)
		}
		definitions = append(definitions, project)
	}
	return definitions, nil
}

func (s *fileStore) Save(project projectDefinition) error {
	if err := s.ensureProjectDir(project.ID); err != nil {
		return err
	}
	data, err := json.MarshalIndent(project, "", "  ")
	if err != nil {
		return fmt.Errorf("encode project definition: %w", err)
	}
	data = append(data, '\n')
	path, err := s.definitionPath(project.ID)
	if err != nil {
		return err
	}
	if err := atomicWrite(path, data); err != nil {
		return fmt.Errorf("write project definition: %w", err)
	}
	return nil
}

func (s *fileStore) Delete(id string) error {
	dir, err := s.projectDir(id)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("delete project directory: %w", err)
	}
	return nil
}

func (s *fileStore) ensureProjectDir(id string) error {
	dir, err := s.projectDir(id)
	if err != nil {
		return err
	}
	for _, path := range []string{s.projectsDir(), dir} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return fmt.Errorf("create project directory %q: %w", path, err)
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return fmt.Errorf("secure project directory %q: %w", path, err)
		}
	}
	return nil
}

func (s *fileStore) projectsDir() string { return filepath.Join(s.stateDir, "projects") }

func (s *fileStore) projectDir(id string) (string, error) {
	if err := validateName(id); err != nil || filepath.Base(id) != id || strings.ContainsAny(id, `/\\`) {
		return "", fmt.Errorf("projects: invalid project id %q", id)
	}
	return filepath.Join(s.projectsDir(), id), nil
}

func (s *fileStore) definitionPath(id string) (string, error) {
	dir, err := s.projectDir(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "project.json"), nil
}

func (s *fileStore) repositoryPath(id string) (string, error) {
	dir, err := s.projectDir(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "repository.git"), nil
}

func atomicWrite(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".project-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
