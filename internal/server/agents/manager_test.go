package agents

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/kurtisvg/ahh/internal/harness"
)

func TestNewManagerCreatesAndReloadsDefaultAgent(t *testing.T) {
	stateDir := t.TempDir()

	manager, err := NewManager(stateDir)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	want := manager.List()
	if len(want) != 1 || want[0].Name != DefaultAgentName || want[0].Harness != harness.ClaudeCode {
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

	first, err := manager.Create("  Build & Test  ", harness.ClaudeCode)
	if err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	second, err := manager.Create("Build Test!", harness.ClaudeCode)
	if err != nil {
		t.Fatalf("Create(second) error = %v", err)
	}
	fallback, err := manager.Create("開発", harness.ClaudeCode)
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

func TestManagerCreateRejectsInvalidConfigs(t *testing.T) {
	tests := []struct {
		name        string
		agentName   string
		harnessType harness.Type
		wantErr     error
	}{
		{
			name:        "blank name",
			agentName:   " \t ",
			harnessType: harness.ClaudeCode,
		},
		{
			name:        "duplicate name",
			agentName:   " default ",
			harnessType: harness.ClaudeCode,
			wantErr:     ErrNameConflict,
		},
		{
			name:        "unsupported harness",
			agentName:   "Codex",
			harnessType: "codex",
			wantErr:     ErrUnsupportedHarness,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(t.TempDir())
			if err != nil {
				t.Fatalf("NewManager() error = %v", err)
			}

			_, err = manager.Create(tt.agentName, tt.harnessType)
			if err == nil {
				t.Fatal("Create() error = nil, want error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("Create() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestManagerUpdatePreservesImmutableFieldsAndConfigDirectory(t *testing.T) {
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
	next := before
	next.Name = "  Primary Claude  "
	updated, err := manager.Update(before.ID, next)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if updated.ID != before.ID {
		t.Errorf("updated ID = %q, want %q", updated.ID, before.ID)
	}
	if updated.Harness != before.Harness {
		t.Errorf("updated harness = %q, want %q", updated.Harness, before.Harness)
	}
	if updated.Name != "Primary Claude" {
		t.Errorf("updated name = %q, want Primary Claude", updated.Name)
	}
	if got, err := manager.ConfigDir(updated.ID); err != nil || got != configDir {
		t.Errorf("ConfigDir() = %q, %v, want %q, nil", got, err, configDir)
	}
	reloaded, err := NewManager(stateDir)
	if err != nil {
		t.Fatalf("reload NewManager() error = %v", err)
	}
	if got, ok := reloaded.Get(updated.ID); !ok || !reflect.DeepEqual(got, updated) {
		t.Errorf("reloaded Get() = %#v, %v, want %#v, true", got, ok, updated)
	}
}

func TestManagerUpdateRejectsInvalidConfigs(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*testing.T, *Manager, Config) (string, Config)
		wantErr error
	}{
		{
			name: "missing agent",
			prepare: func(t *testing.T, _ *Manager, current Config) (string, Config) {
				t.Helper()
				return uuid.NewString(), current
			},
			wantErr: ErrNotFound,
		},
		{
			name: "blank name",
			prepare: func(t *testing.T, _ *Manager, current Config) (string, Config) {
				t.Helper()
				current.Name = " \t "
				return current.ID, current
			},
		},
		{
			name: "duplicate name",
			prepare: func(t *testing.T, manager *Manager, current Config) (string, Config) {
				t.Helper()
				if _, err := manager.Create("Other", harness.ClaudeCode); err != nil {
					t.Fatalf("Create() error = %v", err)
				}
				current.Name = " other "
				return current.ID, current
			},
			wantErr: ErrNameConflict,
		},
		{
			name: "changed id",
			prepare: func(t *testing.T, _ *Manager, current Config) (string, Config) {
				t.Helper()
				targetID := current.ID
				current.ID = uuid.NewString()
				return targetID, current
			},
			wantErr: ErrImmutable,
		},
		{
			name: "changed harness",
			prepare: func(t *testing.T, _ *Manager, current Config) (string, Config) {
				t.Helper()
				current.Harness = "codex"
				return current.ID, current
			},
			wantErr: ErrImmutable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(t.TempDir())
			if err != nil {
				t.Fatalf("NewManager() error = %v", err)
			}
			current := manager.List()[0]
			targetID, next := tt.prepare(t, manager, current)

			_, err = manager.Update(targetID, next)
			if err == nil {
				t.Fatal("Update() error = nil, want error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("Update() error = %v, want %v", err, tt.wantErr)
			}
			if got, ok := manager.Get(current.ID); !ok || !reflect.DeepEqual(got, current) {
				t.Errorf("Get() after rejected update = %#v, %v, want %#v, true", got, ok, current)
			}
		})
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

func TestNewManagerRejectsInvalidConfigs(t *testing.T) {
	const validID = "9e065f6f-3342-4ee3-9443-3c74ec64012d"
	const otherID = "92ec4ca3-1a21-4c50-97c4-a78169ca568f"
	tests := []struct {
		name        string
		directoryID string
		config      Config
		wantErr     error
	}{
		{
			name:        "directory and config id mismatch",
			directoryID: validID,
			config:      Config{ID: otherID, Name: "Different", Harness: harness.ClaudeCode},
		},
		{
			name:        "legacy non-uuid id",
			directoryID: "claude-code",
			config: Config{
				ID:      "claude-code",
				Name:    DefaultAgentName,
				Harness: harness.ClaudeCode,
			},
		},
		{
			name:        "blank name",
			directoryID: validID,
			config:      Config{ID: validID, Name: " ", Harness: harness.ClaudeCode},
		},
		{
			name:        "unsupported harness",
			directoryID: validID,
			config:      Config{ID: validID, Name: "Codex", Harness: "codex"},
			wantErr:     ErrUnsupportedHarness,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stateDir := t.TempDir()
			writeAgentConfig(t, stateDir, tt.directoryID, tt.config)

			_, err := NewManager(stateDir)
			if err == nil {
				t.Fatal("NewManager() error = nil, want error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewManager() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func writeAgentConfig(t *testing.T, stateDir, directoryID string, config Config) {
	t.Helper()

	dir := filepath.Join(stateDir, "agents", directoryID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent.json"), data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
