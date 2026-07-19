package conversations

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

	if err := s.migrateLegacyDir(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(s.conversationsDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Metadata{}, nil
		}

		return nil, fmt.Errorf("read conversations directory: %w", err)
	}

	metadata := make([]Metadata, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(s.conversationsDir(), entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read conversation metadata %q: %w", path, err)
		}

		var conversation Metadata
		if err := json.Unmarshal(data, &conversation); err != nil {
			return nil, fmt.Errorf("decode conversation metadata %q: %w", path, err)
		}
		fileID := strings.TrimSuffix(entry.Name(), ".json")
		if conversation.ID == "" {
			conversation.ID = fileID
		}
		if conversation.ID != fileID {
			return nil, fmt.Errorf(
				"conversation metadata %q has id %q, want %q",
				path,
				conversation.ID,
				fileID,
			)
		}
		if strings.TrimSpace(conversation.AgentID) == "" {
			return nil, fmt.Errorf(
				"conversation metadata %q is missing required agent_id; pre-Agent metadata is not supported",
				path,
			)
		}
		if conversation.LastActiveAt.IsZero() {
			conversation.LastActiveAt = conversation.CreatedAt
		}
		conversation.Status = StatusExited
		metadata = append(metadata, conversation)
	}
	sortByActivity(metadata)

	return metadata, nil
}

func (s *fileMetadataStore) Save(conversation Metadata) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.conversationsDir(), 0o755); err != nil {
		return fmt.Errorf("create conversations directory: %w", err)
	}

	data, err := json.MarshalIndent(conversation, "", "  ")
	if err != nil {
		return fmt.Errorf("encode conversation metadata: %w", err)
	}
	data = append(data, '\n')

	path, err := s.conversationPath(conversation.ID)
	if err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write conversation metadata: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace conversation metadata: %w", err)
	}

	return nil
}

func (s *fileMetadataStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := s.conversationPath(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete conversation metadata: %w", err)
	}

	return nil
}

func (s *fileMetadataStore) conversationsDir() string {
	return filepath.Join(s.stateDir, "conversations")
}

// migrateLegacyDir preserves metadata written before conversations replaced
// sessions in Ahh's product terminology.
func (s *fileMetadataStore) migrateLegacyDir() error {
	if _, err := os.Stat(s.conversationsDir()); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect conversations directory: %w", err)
	}

	legacyDir := filepath.Join(s.stateDir, "sessions")
	if err := os.Rename(legacyDir, s.conversationsDir()); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("migrate legacy sessions directory: %w", err)
	}

	return nil
}

func (s *fileMetadataStore) conversationPath(id string) (string, error) {
	if id == "" || filepath.Base(id) != id || strings.ContainsAny(id, `/\\`) {
		return "", fmt.Errorf("invalid conversation id %q", id)
	}

	return filepath.Join(s.conversationsDir(), id+".json"), nil
}
