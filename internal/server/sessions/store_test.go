package sessions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileMetadataStoreLoadRejectsMismatchedID(t *testing.T) {
	stateDir := t.TempDir()
	sessionsDir := filepath.Join(stateDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("create sessions directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(sessionsDir, "expected.json"),
		[]byte(`{"id":"different","name":"terminal"}`),
		0o644,
	); err != nil {
		t.Fatalf("write session metadata: %v", err)
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
	if !strings.Contains(err.Error(), "invalid session id") {
		t.Fatalf("Save() error = %q, want invalid session ID error", err)
	}
}
