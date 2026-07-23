package projects

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type testEnvironment struct {
	env []string
	err error
}

func (e testEnvironment) GitEnvironment(bool) ([]string, error) {
	return append([]string(nil), e.env...), e.err
}

type fakeCommandRunner struct {
	mu         sync.Mutex
	commands   [][]string
	failFetch  bool
	originURL  string
	remoteHEAD string
	branchRefs string
}

func (r *fakeCommandRunner) Run(ctx context.Context, _ []string, args ...string) ([]byte, error) {
	r.mu.Lock()
	r.commands = append(r.commands, append([]string(nil), args...))
	failFetch := r.failFetch
	originURL := r.originURL
	remoteHEAD := r.remoteHEAD
	branchRefs := r.branchRefs
	r.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	command := strings.Join(args, " ")
	switch {
	case strings.Contains(command, " fetch "):
		if failFetch {
			return []byte("private diagnostic with a filesystem path"), errors.New("exit status 128")
		}
	case strings.Contains(command, "remote get-url origin"):
		return []byte(originURL + "\n"), nil
	case strings.Contains(command, "ls-remote --symref origin HEAD"):
		if remoteHEAD == "" {
			remoteHEAD = "main"
		}
		return []byte("ref: refs/heads/" + remoteHEAD + "\tHEAD\nabc\tHEAD\n"), nil
	case strings.Contains(command, "rev-parse --is-bare-repository"):
		return []byte("true\n"), nil
	case strings.Contains(command, "for-each-ref"):
		return []byte(branchRefs), nil
	}
	return nil, nil
}

func (r *fakeCommandRunner) commandStrings() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	commands := make([]string, len(r.commands))
	for i, command := range r.commands {
		commands[i] = strings.Join(command, " ")
	}
	return commands
}

func TestWithCommandRunnerRejectsNil(t *testing.T) {
	_, err := NewManager(t.TempDir(), testEnvironment{}, WithCommandRunner(nil))
	if err == nil || !strings.Contains(err.Error(), "project command runner must not be nil") {
		t.Fatalf("NewManager() error = %v, want nil command runner error", err)
	}
}

func TestManagerCreatesPersistsAndReloadsProject(t *testing.T) {
	stateDir := t.TempDir()
	runner := &fakeCommandRunner{
		originURL:  "git@github.com:kurtisvg/ahh.git",
		remoteHEAD: "trunk",
		branchRefs: "refs/heads/local-work\nrefs/remotes/origin/HEAD\nrefs/remotes/origin/trunk\n",
	}
	manager, err := NewManager(
		stateDir,
		testEnvironment{env: []string{"GIT_TERMINAL_PROMPT=0"}},
		WithCommandRunner(runner),
	)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	created, err := manager.Create(t.Context(), "Ahh", "kurtisvg/ahh.git")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Status != StatusReady || created.Source.Repository != "kurtisvg/ahh" {
		t.Fatalf("created Project = %+v, want ready normalized Project", created)
	}
	if created.ID != created.Name || created.ID != "Ahh" {
		t.Fatalf("created Project id/name = %q/%q, want Ahh/Ahh", created.ID, created.Name)
	}
	if created.DefaultBranch != (Branch{Kind: BranchRemote, Name: "trunk"}) {
		t.Fatalf("default branch = %+v, want remote trunk", created.DefaultBranch)
	}

	definitionPath := filepath.Join(stateDir, "projects", "Ahh", "project.json")
	data, err := os.ReadFile(definitionPath)
	if err != nil {
		t.Fatalf("read project definition: %v", err)
	}
	if strings.Contains(string(data), "status") || strings.Contains(string(data), "diagnostic") {
		t.Fatalf("persisted definition contains runtime state: %s", data)
	}
	var stored definition
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("decode project definition: %v", err)
	}
	if stored.DefaultBranch.Name != "trunk" {
		t.Fatalf("stored default branch = %+v, want trunk", stored.DefaultBranch)
	}
	assertFileMode(t, definitionPath, 0o600)
	assertFileMode(t, filepath.Dir(definitionPath), 0o700)

	branches, err := manager.Branches(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("Branches() error = %v", err)
	}
	wantBranches := []Branch{{Kind: BranchLocal, Name: "local-work"}, {Kind: BranchRemote, Name: "trunk"}}
	if !reflect.DeepEqual(branches, wantBranches) {
		t.Fatalf("Branches() = %+v, want %+v", branches, wantBranches)
	}
	updated, err := manager.UpdateDefaultBranch(t.Context(), created.ID, Branch{Kind: BranchLocal, Name: "local-work"})
	if err != nil {
		t.Fatalf("UpdateDefaultBranch() error = %v", err)
	}
	if updated.DefaultBranch.Kind != BranchLocal {
		t.Fatalf("updated default = %+v, want local", updated.DefaultBranch)
	}

	reloaded, err := NewManager(stateDir, testEnvironment{}, WithCommandRunner(runner))
	if err != nil {
		t.Fatalf("reload NewManager() error = %v", err)
	}
	listed := reloaded.List()
	if len(listed) != 1 || listed[0].Name != "Ahh" || listed[0].DefaultBranch.Kind != BranchLocal {
		t.Fatalf("reloaded Projects = %+v", listed)
	}
	commands := strings.Join(runner.commandStrings(), "\n")
	for _, expected := range []string{
		"init --bare " + filepath.Join(stateDir, "projects", "Ahh", "repository.git"),
		"fetch --prune origin +refs/heads/*:refs/remotes/origin/*",
		"symbolic-ref refs/remotes/origin/HEAD refs/remotes/origin/trunk",
	} {
		if !strings.Contains(commands, expected) {
			t.Fatalf("commands = %q, want containing %q", commands, expected)
		}
	}
}

func TestManagerProjectUniquenessAndDuplicateRepository(t *testing.T) {
	runner := &fakeCommandRunner{originURL: "git@github.com:owner/repo.git", branchRefs: "refs/remotes/origin/main\n"}
	manager, err := NewManager(t.TempDir(), testEnvironment{}, WithCommandRunner(runner))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if _, err := manager.Create(t.Context(), "First", "owner/repo"); err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	if _, err := manager.Create(t.Context(), "first", "owner/repo"); !errors.Is(err, ErrNameExists) {
		t.Fatalf("Create(duplicate name) error = %v, want ErrNameExists", err)
	}
	if _, err := manager.Create(t.Context(), "Second", "owner/repo"); err != nil {
		t.Fatalf("Create(duplicate repository) error = %v", err)
	}
	if got := len(manager.List()); got != 2 {
		t.Fatalf("Projects = %d, want 2", got)
	}
}

func TestManagerRequiresURLSafeProjectName(t *testing.T) {
	manager, err := NewManager(t.TempDir(), testEnvironment{}, WithCommandRunner(&fakeCommandRunner{}))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	tests := []struct {
		name  string
		value string
		err   error
	}{
		{name: "empty", value: "", err: ErrNameRequired},
		{name: "whitespace", value: "my project", err: ErrNameInvalid},
		{name: "path separator", value: "owner/project", err: ErrNameInvalid},
		{name: "path traversal", value: "..", err: ErrNameInvalid},
		{name: "leading punctuation", value: "-project", err: ErrNameInvalid},
		{name: "trailing punctuation", value: "project-", err: ErrNameInvalid},
		{name: "too long", value: strings.Repeat("a", 65), err: ErrNameInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := manager.Create(t.Context(), tt.value, "owner/repo"); !errors.Is(err, tt.err) {
				t.Fatalf("Create(%q) error = %v, want %v", tt.value, err, tt.err)
			}
		})
	}
}

func TestManagerReportsSanitizedFetchFailure(t *testing.T) {
	runner := &fakeCommandRunner{originURL: "git@github.com:owner/private.git", failFetch: true}
	manager, err := NewManager(t.TempDir(), testEnvironment{}, WithCommandRunner(runner))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	created, err := manager.Create(t.Context(), "Private", "owner/private")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Status != StatusUnavailable || created.Diagnostic != diagnosticAccess {
		t.Fatalf("created Project = %+v, want sanitized unavailable status", created)
	}
	if strings.Contains(created.Diagnostic, "filesystem") || strings.Contains(created.Diagnostic, "exit status") {
		t.Fatalf("diagnostic leaked technical details: %q", created.Diagnostic)
	}
	if _, err := manager.Fetch(t.Context(), created.ID); err == nil {
		t.Fatal("Fetch() error = nil, want failure")
	}
}

func TestManagerBlocksManagedOperationsWhenIdentityInvalid(t *testing.T) {
	manager, err := NewManager(t.TempDir(), testEnvironment{err: errors.New("invalid identity")}, WithCommandRunner(&fakeCommandRunner{}))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	created, err := manager.Create(t.Context(), "Blocked", "owner/repo")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Status != StatusUnavailable || created.Diagnostic != diagnosticAccess {
		t.Fatalf("blocked Project = %+v", created)
	}
}

func TestMergeEnvironmentOverridesExistingValues(t *testing.T) {
	actual := mergeEnvironment([]string{"A=one", "GIT_SSH_COMMAND=ambient", "B=two"}, []string{"GIT_SSH_COMMAND=managed", "C=three"})
	want := []string{"A=one", "B=two", "GIT_SSH_COMMAND=managed", "C=three"}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("mergeEnvironment() = %q, want %q", actual, want)
	}
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode %q = %o, want %o", path, got, want)
	}
}
