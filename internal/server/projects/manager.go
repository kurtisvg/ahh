package projects

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var (
	ErrNameRequired   = errors.New("projects: name is required")
	ErrNameInvalid    = errors.New("projects: name is not a url-safe identifier")
	ErrNameExists     = errors.New("projects: name already exists")
	ErrNotFound       = errors.New("projects: project not found")
	ErrBranchNotFound = errors.New("projects: branch not found")
	ErrDeleting       = errors.New("projects: project is being deleted")
)

const (
	diagnosticAccess         = "GitHub repository access failed. Check the SSH key registration and repository permissions."
	diagnosticInitialization = "The managed Git repository could not be initialized."
	diagnosticInvalid        = "The managed Git repository is invalid."
	diagnosticTimeout        = "The Git operation timed out."
)

// Project serializes repository operations and owns one persisted definition.
type Project struct {
	operationMu   sync.Mutex
	definition    definition
	status        Status
	diagnostic    string
	isDeleting    bool
	hasRemoteRefs bool

	store          *fileStore
	gitEnvironment gitEnvironment
	runner         commandRunner
}

// Manager owns the installation Project registry.
type Manager struct {
	mu             sync.Mutex
	projects       map[string]*Project
	names          map[string]string
	store          *fileStore
	gitEnvironment gitEnvironment
	runner         commandRunner
}

type options struct {
	runner commandRunner
}

// Option configures a Project manager.
type Option func(*options) error

// NewManager loads Projects beneath stateDir and inspects their local repository state.
func NewManager(stateDir string, environment gitEnvironment, opts ...Option) (*Manager, error) {
	if strings.TrimSpace(stateDir) == "" {
		return nil, fmt.Errorf("project state directory is required")
	}
	if environment == nil {
		return nil, fmt.Errorf("project git environment is required")
	}
	cfg := options{runner: execRunner{}}
	for _, opt := range opts {
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}
	store := newFileStore(stateDir)
	definitions, err := store.Load()
	if err != nil {
		return nil, err
	}
	m := &Manager{
		projects:       make(map[string]*Project, len(definitions)),
		names:          make(map[string]string, len(definitions)),
		store:          store,
		gitEnvironment: environment,
		runner:         cfg.runner,
	}
	for _, stored := range definitions {
		nameKey := normalizeName(stored.Name)
		if existing, ok := m.names[nameKey]; ok {
			return nil, fmt.Errorf("projects %q and %q have duplicate names", existing, stored.ID)
		}
		project := m.newProject(stored)
		project.inspect(context.Background())
		m.projects[stored.ID] = project
		m.names[nameKey] = stored.ID
	}
	return m, nil
}

func (m *Manager) newProject(stored definition) *Project {
	return &Project{
		definition:     stored,
		status:         StatusUnavailable,
		diagnostic:     diagnosticInvalid,
		store:          m.store,
		gitEnvironment: m.gitEnvironment,
		runner:         m.runner,
	}
}

// WithCommandRunner replaces Git process execution for tests.
func WithCommandRunner(runner commandRunner) Option {
	return func(opts *options) error {
		if runner == nil {
			return fmt.Errorf("project command runner is required")
		}
		opts.runner = runner
		return nil
	}
}

// Create persists a Project and attempts to initialize its managed repository.
// Repository access failures produce an unavailable Project that can be retried.
func (m *Manager) Create(ctx context.Context, name, repository string) (Metadata, error) {
	if err := validateName(name); err != nil {
		return Metadata{}, err
	}
	repository, err := NormalizeGitHubRepository(repository)
	if err != nil {
		return Metadata{}, err
	}
	stored := definition{
		ID:            name,
		Name:          name,
		Source:        Source{Type: SourceGitHub, Repository: repository},
		DefaultBranch: Branch{Kind: BranchRemote, Name: "main"},
	}
	project := m.newProject(stored)

	m.mu.Lock()
	nameKey := normalizeName(name)
	if _, exists := m.names[nameKey]; exists {
		m.mu.Unlock()
		return Metadata{}, ErrNameExists
	}
	if err := m.store.Save(stored); err != nil {
		m.mu.Unlock()
		return Metadata{}, err
	}
	m.projects[name] = project
	m.names[nameKey] = name
	m.mu.Unlock()

	_ = project.initialize(ctx)
	return project.Metadata(), nil
}

// List returns Projects sorted case-insensitively by name.
func (m *Manager) List() []Metadata {
	m.mu.Lock()
	projects := make([]*Project, 0, len(m.projects))
	for _, project := range m.projects {
		projects = append(projects, project)
	}
	m.mu.Unlock()
	metadata := make([]Metadata, 0, len(projects))
	for _, project := range projects {
		metadata = append(metadata, project.Metadata())
	}
	sort.Slice(metadata, func(i, j int) bool {
		return normalizeName(metadata[i].Name) < normalizeName(metadata[j].Name)
	})
	return metadata
}

// Get returns a Project by immutable ID.
func (m *Manager) Get(id string) (*Project, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	project, ok := m.projects[id]
	return project, ok
}

// UpdateDefaultBranch validates and persists a Project default branch.
func (m *Manager) UpdateDefaultBranch(ctx context.Context, id string, branch Branch) (Metadata, error) {
	project, ok := m.Get(id)
	if !ok {
		return Metadata{}, ErrNotFound
	}
	return project.UpdateDefaultBranch(ctx, branch)
}

// Fetch refreshes and prunes a Project's origin branches.
func (m *Manager) Fetch(ctx context.Context, id string) (Metadata, error) {
	project, ok := m.Get(id)
	if !ok {
		return Metadata{}, ErrNotFound
	}
	err := project.Fetch(ctx)
	return project.Metadata(), err
}

// Branches lists local and origin branches for a Project.
func (m *Manager) Branches(ctx context.Context, id string) ([]Branch, error) {
	project, ok := m.Get(id)
	if !ok {
		return nil, ErrNotFound
	}
	return project.Branches(ctx)
}

// Delete permanently removes a Project that has no Conversation integration yet.
func (m *Manager) Delete(id string) (bool, error) {
	m.mu.Lock()
	project, ok := m.projects[id]
	if !ok {
		m.mu.Unlock()
		return false, nil
	}
	project.operationMu.Lock()
	if project.isDeleting {
		project.operationMu.Unlock()
		m.mu.Unlock()
		return false, ErrDeleting
	}
	project.isDeleting = true
	m.mu.Unlock()

	err := m.store.Delete(id)
	if err != nil {
		project.isDeleting = false
		project.operationMu.Unlock()
		return false, err
	}
	m.mu.Lock()
	delete(m.projects, id)
	delete(m.names, normalizeName(project.definition.Name))
	m.mu.Unlock()
	project.operationMu.Unlock()
	return true, nil
}

// Metadata returns a concurrency-safe Project snapshot.
func (p *Project) Metadata() Metadata {
	p.operationMu.Lock()
	defer p.operationMu.Unlock()
	return p.metadataLocked()
}

func (p *Project) metadataLocked() Metadata {
	return Metadata{
		ID:            p.definition.ID,
		Name:          p.definition.Name,
		Source:        p.definition.Source,
		DefaultBranch: p.definition.DefaultBranch,
		Status:        p.status,
		Diagnostic:    p.diagnostic,
	}
}

func (p *Project) initialize(ctx context.Context) error {
	p.operationMu.Lock()
	defer p.operationMu.Unlock()
	if p.isDeleting {
		return ErrDeleting
	}

	repositoryPath, err := p.store.repositoryPath(p.definition.ID)
	if err != nil {
		return p.unavailableLocked(err, diagnosticInitialization)
	}
	if _, err := os.Stat(repositoryPath); errors.Is(err, os.ErrNotExist) {
		env, envErr := p.gitEnvironment.GitEnvironment(true)
		if envErr != nil {
			return p.unavailableLocked(envErr, diagnosticAccess)
		}
		if _, err := runWithTimeout(ctx, localOperationTimeout, p.runner, env, "init", "--bare", repositoryPath); err != nil {
			return p.unavailableLocked(err, diagnosticInitialization)
		}
		source, err := sourceFor(p.definition.Source)
		if err != nil {
			return p.unavailableLocked(err, diagnosticInitialization)
		}
		if _, err := runWithTimeout(ctx, localOperationTimeout, p.runner, env, "--git-dir", repositoryPath, "remote", "add", "origin", source.SSHURL()); err != nil {
			return p.unavailableLocked(err, diagnosticInitialization)
		}
	} else if err != nil {
		return p.unavailableLocked(err, diagnosticInitialization)
	}
	return p.fetchLocked(ctx, true)
}

func (p *Project) inspect(ctx context.Context) {
	p.operationMu.Lock()
	defer p.operationMu.Unlock()
	env, err := p.gitEnvironment.GitEnvironment(true)
	if err != nil {
		_ = p.unavailableLocked(err, diagnosticAccess)
		return
	}
	repositoryPath, err := p.store.repositoryPath(p.definition.ID)
	if err != nil {
		_ = p.unavailableLocked(err, diagnosticInvalid)
		return
	}
	output, err := runWithTimeout(ctx, localOperationTimeout, p.runner, env, "--git-dir", repositoryPath, "rev-parse", "--is-bare-repository")
	if err != nil || strings.TrimSpace(string(output)) != "true" {
		_ = p.unavailableLocked(err, diagnosticInvalid)
		return
	}
	source, err := sourceFor(p.definition.Source)
	if err != nil {
		_ = p.unavailableLocked(err, diagnosticInvalid)
		return
	}
	output, err = runWithTimeout(ctx, localOperationTimeout, p.runner, env, "--git-dir", repositoryPath, "remote", "get-url", "origin")
	if err != nil || strings.TrimSpace(string(output)) != source.SSHURL() {
		_ = p.unavailableLocked(err, diagnosticInvalid)
		return
	}
	branches, err := p.branchesLocked(ctx, env)
	if err != nil || len(branches) == 0 {
		_ = p.unavailableLocked(err, diagnosticInvalid)
		return
	}
	p.hasRemoteRefs = hasRemoteBranches(branches)
	p.status = StatusReady
	p.diagnostic = ""
}

// Fetch refreshes origin and reports a sanitized unavailable state on failure.
func (p *Project) Fetch(ctx context.Context) error {
	p.operationMu.Lock()
	defer p.operationMu.Unlock()
	if p.isDeleting {
		return ErrDeleting
	}
	repositoryPath, err := p.store.repositoryPath(p.definition.ID)
	if err != nil {
		return p.unavailableLocked(err, diagnosticInitialization)
	}
	if _, err := os.Stat(repositoryPath); errors.Is(err, os.ErrNotExist) {
		env, envErr := p.gitEnvironment.GitEnvironment(true)
		if envErr != nil {
			return p.unavailableLocked(envErr, diagnosticAccess)
		}
		if _, err := runWithTimeout(ctx, localOperationTimeout, p.runner, env, "init", "--bare", repositoryPath); err != nil {
			return p.unavailableLocked(err, diagnosticInitialization)
		}
	} else if err != nil {
		return p.unavailableLocked(err, diagnosticInitialization)
	}
	return p.fetchLocked(ctx, !p.hasRemoteRefs)
}

func (p *Project) fetchLocked(ctx context.Context, detectDefault bool) error {
	env, err := p.gitEnvironment.GitEnvironment(true)
	if err != nil {
		return p.unavailableLocked(err, diagnosticAccess)
	}
	repositoryPath, err := p.store.repositoryPath(p.definition.ID)
	if err != nil {
		return p.unavailableLocked(err, diagnosticInitialization)
	}
	source, err := sourceFor(p.definition.Source)
	if err != nil {
		return p.unavailableLocked(err, diagnosticInitialization)
	}
	remoteURL, remoteErr := runWithTimeout(ctx, localOperationTimeout, p.runner, env,
		"--git-dir", repositoryPath, "remote", "get-url", "origin",
	)
	if remoteErr != nil {
		if _, err := runWithTimeout(ctx, localOperationTimeout, p.runner, env,
			"--git-dir", repositoryPath, "remote", "add", "origin", source.SSHURL(),
		); err != nil {
			return p.unavailableLocked(err, diagnosticInitialization)
		}
	} else if strings.TrimSpace(string(remoteURL)) != source.SSHURL() {
		return p.unavailableLocked(errors.New("projects: origin does not match source"), diagnosticInvalid)
	}
	if _, err := runWithTimeout(ctx, remoteOperationTimeout, p.runner, env,
		"--git-dir", repositoryPath, "fetch", "--prune", "origin", "+refs/heads/*:refs/remotes/origin/*",
	); err != nil {
		return p.unavailableLocked(err, diagnosticForGitError(err, true))
	}
	defaultName, err := p.remoteHEAD(ctx, env, repositoryPath)
	if err != nil {
		return p.unavailableLocked(err, diagnosticForGitError(err, true))
	}
	if _, err := runWithTimeout(ctx, localOperationTimeout, p.runner, env,
		"--git-dir", repositoryPath, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/"+defaultName,
	); err != nil {
		return p.unavailableLocked(err, diagnosticInvalid)
	}
	if detectDefault {
		p.definition.DefaultBranch = Branch{Kind: BranchRemote, Name: defaultName}
		if err := p.store.Save(p.definition); err != nil {
			return p.unavailableLocked(err, diagnosticInitialization)
		}
	}
	p.hasRemoteRefs = true
	p.status = StatusReady
	p.diagnostic = ""
	return nil
}

func (p *Project) remoteHEAD(ctx context.Context, env []string, repositoryPath string) (string, error) {
	output, err := runWithTimeout(ctx, remoteOperationTimeout, p.runner, env,
		"--git-dir", repositoryPath, "ls-remote", "--symref", "origin", "HEAD",
	)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "ref:" && fields[2] == "HEAD" && strings.HasPrefix(fields[1], "refs/heads/") {
			name := strings.TrimPrefix(fields[1], "refs/heads/")
			if name != "" {
				return name, nil
			}
		}
	}
	return "", fmt.Errorf("projects: remote head is unavailable")
}

// Branches returns local branches followed by origin branches, sorted by name.
func (p *Project) Branches(ctx context.Context) ([]Branch, error) {
	p.operationMu.Lock()
	defer p.operationMu.Unlock()
	if p.isDeleting {
		return nil, ErrDeleting
	}
	env, err := p.gitEnvironment.GitEnvironment(true)
	if err != nil {
		return nil, p.unavailableLocked(err, diagnosticAccess)
	}
	return p.branchesLocked(ctx, env)
}

func (p *Project) branchesLocked(ctx context.Context, env []string) ([]Branch, error) {
	repositoryPath, err := p.store.repositoryPath(p.definition.ID)
	if err != nil {
		return nil, err
	}
	output, err := runWithTimeout(ctx, localOperationTimeout, p.runner, env,
		"--git-dir", repositoryPath, "for-each-ref", "--format=%(refname)", "refs/heads", "refs/remotes/origin",
	)
	if err != nil {
		return nil, err
	}
	branches := make([]Branch, 0)
	for _, ref := range strings.Fields(string(output)) {
		switch {
		case strings.HasPrefix(ref, "refs/heads/"):
			branches = append(branches, Branch{Kind: BranchLocal, Name: strings.TrimPrefix(ref, "refs/heads/")})
		case strings.HasPrefix(ref, "refs/remotes/origin/") && ref != "refs/remotes/origin/HEAD":
			branches = append(branches, Branch{Kind: BranchRemote, Name: strings.TrimPrefix(ref, "refs/remotes/origin/")})
		}
	}
	sort.Slice(branches, func(i, j int) bool {
		if branches[i].Kind == branches[j].Kind {
			return branches[i].Name < branches[j].Name
		}
		return branches[i].Kind == BranchLocal
	})
	return branches, nil
}

// UpdateDefaultBranch validates an existing local/origin branch and persists it.
func (p *Project) UpdateDefaultBranch(ctx context.Context, branch Branch) (Metadata, error) {
	if err := validateBranch(branch); err != nil {
		return Metadata{}, err
	}
	p.operationMu.Lock()
	defer p.operationMu.Unlock()
	if p.isDeleting {
		return Metadata{}, ErrDeleting
	}
	env, err := p.gitEnvironment.GitEnvironment(true)
	if err != nil {
		return Metadata{}, p.unavailableLocked(err, diagnosticAccess)
	}
	repositoryPath, err := p.store.repositoryPath(p.definition.ID)
	if err != nil {
		return Metadata{}, err
	}
	if _, err := runWithTimeout(ctx, localOperationTimeout, p.runner, env, "check-ref-format", "--branch", branch.Name); err != nil {
		return Metadata{}, fmt.Errorf("projects: invalid branch name: %w", err)
	}
	ref := "refs/heads/" + branch.Name
	if branch.Kind == BranchRemote {
		ref = "refs/remotes/origin/" + branch.Name
	}
	if _, err := runWithTimeout(ctx, localOperationTimeout, p.runner, env,
		"--git-dir", repositoryPath, "show-ref", "--verify", "--quiet", ref,
	); err != nil {
		return Metadata{}, ErrBranchNotFound
	}
	p.definition.DefaultBranch = branch
	if err := p.store.Save(p.definition); err != nil {
		return Metadata{}, err
	}
	return p.metadataLocked(), nil
}

func (p *Project) unavailableLocked(err error, diagnostic string) error {
	p.status = StatusUnavailable
	p.diagnostic = diagnostic
	if err == nil {
		return errors.New("projects: repository unavailable")
	}
	return err
}

func diagnosticForGitError(err error, remote bool) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return diagnosticTimeout
	}
	if remote {
		return diagnosticAccess
	}
	return diagnosticInvalid
}

func hasRemoteBranches(branches []Branch) bool {
	for _, branch := range branches {
		if branch.Kind == BranchRemote {
			return true
		}
	}
	return false
}

func normalizeName(name string) string { return strings.ToLower(name) }

func (p *Project) repositoryPath() string {
	path, _ := p.store.repositoryPath(p.definition.ID)
	return filepath.Clean(path)
}
