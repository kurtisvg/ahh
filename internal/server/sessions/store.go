package sessions

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type metadataStore interface {
	Load() ([]Metadata, error)
	Save(Metadata) error
	Delete(id string) error
}

type fileMetadataStore struct {
	// mu serializes file operations rooted at stateDir.
	mu       sync.Mutex
	stateDir string
}

func newFileMetadataStore(stateDir string) *fileMetadataStore {
	return &fileMetadataStore{
		stateDir: stateDir,
	}
}

func (s *fileMetadataStore) Load() ([]Metadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.sessionsDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Metadata{}, nil
		}

		return nil, fmt.Errorf("read sessions directory: %w", err)
	}

	metadata := make([]Metadata, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(s.sessionsDir(), entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read session metadata %q: %w", path, err)
		}

		var session Metadata
		if err := json.Unmarshal(data, &session); err != nil {
			return nil, fmt.Errorf("decode session metadata %q: %w", path, err)
		}
		fileID := strings.TrimSuffix(entry.Name(), ".json")
		if session.ID == "" {
			session.ID = fileID
		}
		if session.ID != fileID {
			return nil, fmt.Errorf(
				"session metadata %q has id %q, want %q",
				path,
				session.ID,
				fileID,
			)
		}
		if session.LastActiveAt.IsZero() {
			session.LastActiveAt = session.CreatedAt
		}
		session.Status = StatusExited
		metadata = append(metadata, session)
	}
	sortByActivity(metadata)

	return metadata, nil
}

func (s *fileMetadataStore) Save(session Metadata) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.sessionsDir(), 0o755); err != nil {
		return fmt.Errorf("create sessions directory: %w", err)
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("encode session metadata: %w", err)
	}
	data = append(data, '\n')

	path, err := s.sessionPath(session.ID)
	if err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write session metadata: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace session metadata: %w", err)
	}

	return nil
}

func (s *fileMetadataStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := s.sessionPath(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete session metadata: %w", err)
	}

	return nil
}

func (s *fileMetadataStore) sessionsDir() string {
	return filepath.Join(s.stateDir, "sessions")
}

func (s *fileMetadataStore) sessionPath(id string) (string, error) {
	if id == "" || filepath.Base(id) != id || strings.ContainsAny(id, `/\\`) {
		return "", fmt.Errorf("invalid session id %q", id)
	}

	return filepath.Join(s.sessionsDir(), id+".json"), nil
}
