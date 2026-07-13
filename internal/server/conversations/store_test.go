package conversations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileMetadataStoreLoadRejectsMismatchedID(t *testing.T) {
	stateDir := t.TempDir()
	conversationsDir := filepath.Join(stateDir, "conversations")
	if err := os.MkdirAll(conversationsDir, 0o755); err != nil {
		t.Fatalf("create conversations directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(conversationsDir, "expected.json"),
		[]byte(`{"id":"different","name":"terminal"}`),
		0o644,
	); err != nil {
		t.Fatalf("write conversation metadata: %v", err)
	}

	_, err := newFileMetadataStore(stateDir).Load()
	if err == nil {
		t.Fatal("Load() error = nil, want mismatched ID error")
	}
	if !strings.Contains(err.Error(), `has id "different", want "expected"`) {
		t.Fatalf("Load() error = %q, want mismatched ID details", err)
	}
}

func TestFileMetadataStoreRejectsInvalidID(t *testing.T) {
	store := newFileMetadataStore(t.TempDir())

	err := store.Save(Metadata{ID: "../outside", Name: "terminal"})
	if err == nil {
		t.Fatal("Save() error = nil, want invalid ID error")
	}
	if !strings.Contains(err.Error(), "invalid conversation id") {
		t.Fatalf("Save() error = %q, want invalid conversation ID error", err)
	}
}

func TestFileMetadataStoreMigratesLegacySessionsDirectory(t *testing.T) {
	stateDir := t.TempDir()
	legacyDir := filepath.Join(stateDir, "sessions")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("create legacy sessions directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(legacyDir, "persisted.json"),
		[]byte(`{"id":"persisted","name":"terminal"}`),
		0o644,
	); err != nil {
		t.Fatalf("write legacy metadata: %v", err)
	}

	metadata, err := newFileMetadataStore(stateDir).Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(metadata) != 1 || metadata[0].ID != "persisted" {
		t.Fatalf("Load() metadata = %+v, want persisted conversation", metadata)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "conversations", "persisted.json")); err != nil {
		t.Fatalf("stat migrated conversation metadata: %v", err)
	}
	if _, err := os.Stat(legacyDir); !os.IsNotExist(err) {
		t.Fatalf("stat legacy sessions directory error = %v, want not exist", err)
	}
}
