package agents

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNewManagerCreatesAndReloadsDefaultAgent(t *testing.T) {
	stateDir := t.TempDir()

	manager, err := NewManager(stateDir)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	want := []Config{{
		ID:      "claude-code",
		Name:    "Claude Code",
		Harness: "claude-code",
	}}
	if got := manager.List(); !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %#v, want %#v", got, want)
	}

	reloaded, err := NewManager(stateDir)
	if err != nil {
		t.Fatalf("reload NewManager() error = %v", err)
	}
	if got := reloaded.List(); !reflect.DeepEqual(got, want) {
		t.Fatalf("reloaded List() = %#v, want %#v", got, want)
	}
}

func TestManagerCreateGeneratesStableUniqueIDsAndSortsByName(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	first, err := manager.Create("  Build & Test  ", ClaudeCodeHarness)
	if err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	second, err := manager.Create("Build Test!", ClaudeCodeHarness)
	if err != nil {
		t.Fatalf("Create(second) error = %v", err)
	}
	fallback, err := manager.Create("開発", ClaudeCodeHarness)
	if err != nil {
		t.Fatalf("Create(fallback) error = %v", err)
	}

	if first.ID != "build-test" {
		t.Errorf("first ID = %q, want build-test", first.ID)
	}
	if second.ID != "build-test-2" {
		t.Errorf("second ID = %q, want build-test-2", second.ID)
	}
	if fallback.ID != "agent" {
		t.Errorf("fallback ID = %q, want agent", fallback.ID)
	}

	agents := manager.List()
	wantNames := []string{"Build & Test", "Build Test!", "Claude Code", "開発"}
	for i, want := range wantNames {
		if agents[i].Name != want {
			t.Fatalf("List()[%d].Name = %q, want %q; full list: %#v", i, agents[i].Name, want, agents)
		}
	}
}

func TestManagerRejectsDuplicateNamesAndUnsupportedHarnesses(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if _, err := manager.Create(" claude code ", ClaudeCodeHarness); !errors.Is(err, ErrNameConflict) {
		t.Fatalf("Create(duplicate) error = %v, want ErrNameConflict", err)
	}
	if _, err := manager.Create("Codex", "codex"); !errors.Is(err, ErrUnsupportedHarness) {
		t.Fatalf("Create(unsupported) error = %v, want ErrUnsupportedHarness", err)
	}
}

func TestManagerRenamePreservesIDHarnessAndConfigDirectory(t *testing.T) {
	stateDir := t.TempDir()
	manager, err := NewManager(stateDir)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	before, ok := manager.Get(ClaudeCodeHarness)
	if !ok {
		t.Fatal("default Agent was not found")
	}
	configDir, err := manager.ConfigDir(before.ID)
	if err != nil {
		t.Fatalf("ConfigDir() error = %v", err)
	}
	updated, err := manager.Rename(before.ID, "Primary Claude")
	if err != nil {
		t.Fatalf("Rename() error = %v", err)
	}

	if updated.ID != before.ID {
		t.Errorf("renamed ID = %q, want %q", updated.ID, before.ID)
	}
	if updated.Harness != before.Harness {
		t.Errorf("renamed harness = %q, want %q", updated.Harness, before.Harness)
	}
	if got, err := manager.ConfigDir(updated.ID); err != nil || got != configDir {
		t.Errorf("ConfigDir() = %q, %v, want %q, nil", got, err, configDir)
	}
}

func TestAgentFilesAndDirectoriesUsePrivatePermissions(t *testing.T) {
	stateDir := t.TempDir()
	if _, err := NewManager(stateDir); err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	paths := []struct {
		path string
		mode os.FileMode
	}{
		{filepath.Join(stateDir, "agents"), 0o700},
		{filepath.Join(stateDir, "agents", ClaudeCodeHarness), 0o700},
		{filepath.Join(stateDir, "agents", ClaudeCodeHarness, "config"), 0o700},
		{filepath.Join(stateDir, "agents", ClaudeCodeHarness, "agent.json"), 0o600},
	}
	for _, item := range paths {
		info, err := os.Stat(item.path)
		if err != nil {
			t.Fatalf("Stat(%q) error = %v", item.path, err)
		}
		if got := info.Mode().Perm(); got != item.mode {
			t.Errorf("mode for %q = %o, want %o", item.path, got, item.mode)
		}
	}
}

func TestNewManagerRejectsDirectoryAndConfigIDMismatch(t *testing.T) {
	stateDir := t.TempDir()
	dir := filepath.Join(stateDir, "agents", "expected")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	data, err := json.Marshal(Config{ID: "different", Name: "Different", Harness: ClaudeCodeHarness})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent.json"), data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := NewManager(stateDir); err == nil {
		t.Fatal("NewManager() error = nil, want ID mismatch")
	}
}
