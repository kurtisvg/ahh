package agents

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func TestNewManagerCreatesAndReloadsDefaultAgent(t *testing.T) {
	stateDir := t.TempDir()

	manager, err := NewManager(stateDir)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	want := manager.List()
	if len(want) != 1 || want[0].Name != DefaultAgentName || want[0].Harness != ClaudeCodeHarness {
		t.Fatalf("List() = %#v, want one default Claude Code Agent", want)
	}
	if id, err := uuid.Parse(want[0].ID); err != nil || id.Version() != 4 {
		t.Fatalf("default Agent ID = %q, want UUID v4", want[0].ID)
	}

	reloaded, err := NewManager(stateDir)
	if err != nil {
		t.Fatalf("reload NewManager() error = %v", err)
	}
	if got := reloaded.List(); !reflect.DeepEqual(got, want) {
		t.Fatalf("reloaded List() = %#v, want %#v", got, want)
	}
}

func TestManagerCreateGeneratesStableUUIDsAndSortsByName(t *testing.T) {
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

	for _, agent := range []Config{first, second, fallback} {
		id, parseErr := uuid.Parse(agent.ID)
		if parseErr != nil || id.Version() != 4 {
			t.Errorf("Agent ID = %q, want UUID v4", agent.ID)
		}
	}
	if first.ID == second.ID || first.ID == fallback.ID || second.ID == fallback.ID {
		t.Fatalf("created Agent IDs are not unique: %q, %q, %q", first.ID, second.ID, fallback.ID)
	}

	agents := manager.List()
	wantNames := []string{"Build & Test", "Build Test!", DefaultAgentName, "開発"}
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

	if _, err := manager.Create(" default ", ClaudeCodeHarness); !errors.Is(err, ErrNameConflict) {
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

	listed := manager.List()
	if len(listed) != 1 {
		t.Fatalf("List() = %#v, want one default Agent", listed)
	}
	before, ok := manager.Get(listed[0].ID)
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
	manager, err := NewManager(stateDir)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	listed := manager.List()
	if len(listed) != 1 {
		t.Fatalf("List() = %#v, want one default Agent", listed)
	}
	agentID := listed[0].ID
	paths := []struct {
		path string
		mode os.FileMode
	}{
		{filepath.Join(stateDir, "agents"), 0o700},
		{filepath.Join(stateDir, "agents", agentID), 0o700},
		{filepath.Join(stateDir, "agents", agentID, "config"), 0o700},
		{filepath.Join(stateDir, "agents", agentID, "agent.json"), 0o600},
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
	const expectedID = "9e065f6f-3342-4ee3-9443-3c74ec64012d"
	const differentID = "92ec4ca3-1a21-4c50-97c4-a78169ca568f"
	dir := filepath.Join(stateDir, "agents", expectedID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	data, err := json.Marshal(Config{ID: differentID, Name: "Different", Harness: ClaudeCodeHarness})
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

func TestNewManagerRejectsLegacyNonUUIDAgentIDs(t *testing.T) {
	stateDir := t.TempDir()
	dir := filepath.Join(stateDir, "agents", "claude-code")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	data, err := json.Marshal(Config{ID: "claude-code", Name: DefaultAgentName, Harness: ClaudeCodeHarness})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent.json"), data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := NewManager(stateDir); err == nil {
		t.Fatal("NewManager() error = nil, want invalid legacy Agent ID")
	}
}
