package projects

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	SourceGitHub SourceType = "github"

	BranchLocal  BranchKind = "local"
	BranchRemote BranchKind = "remote"

	StatusReady       Status = "ready"
	StatusUnavailable Status = "unavailable"
)

var (
	githubOwnerPattern       = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)
	projectNamePattern       = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]{0,62}[A-Za-z0-9])?$`)
	repositorySegmentPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

func validateName(name string) error {
	if name == "" {
		return ErrNameRequired
	}
	if !projectNamePattern.MatchString(name) {
		return ErrNameInvalid
	}
	return nil
}

// SourceType identifies a repository source implementation.
type SourceType string

// BranchKind identifies whether a branch is local or tracked from origin.
type BranchKind string

// Status reports whether the managed repository is currently usable.
type Status string

// Source is the immutable public source definition for a Project.
type Source struct {
	Type       SourceType `json:"type"`
	Repository string     `json:"repository"`
}

// Branch identifies a local branch or an origin branch.
type Branch struct {
	Kind BranchKind `json:"kind"`
	Name string     `json:"name"`
}

// Metadata is the complete public representation of a Project.
type Metadata struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Source        Source `json:"source"`
	DefaultBranch Branch `json:"default_branch"`
	Status        Status `json:"status"`
	Diagnostic    string `json:"diagnostic,omitempty"`
}

type definition struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Source        Source `json:"source"`
	DefaultBranch Branch `json:"default_branch"`
}

type repositorySource interface {
	SSHURL() string
}

type githubSource struct {
	repository string
}

func (s githubSource) SSHURL() string {
	return "git@github.com:" + s.repository + ".git"
}

func sourceFor(source Source) (repositorySource, error) {
	if source.Type != SourceGitHub {
		return nil, fmt.Errorf("projects: unsupported source type %q", source.Type)
	}
	repository, err := NormalizeGitHubRepository(source.Repository)
	if err != nil {
		return nil, err
	}
	return githubSource{repository: repository}, nil
}

// NormalizeGitHubRepository validates and normalizes owner/repository input.
func NormalizeGitHubRepository(input string) (string, error) {
	if input == "" || strings.TrimSpace(input) != input || strings.ContainsAny(input, "\x00\r\n\t ") {
		return "", fmt.Errorf("projects: github repository must be owner/repository")
	}
	if strings.HasSuffix(input, ".git") {
		input = strings.TrimSuffix(input, ".git")
	}
	segments := strings.Split(input, "/")
	if len(segments) != 2 {
		return "", fmt.Errorf("projects: github repository must contain exactly two segments")
	}
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." || !repositorySegmentPattern.MatchString(segment) {
			return "", fmt.Errorf("projects: invalid github repository segment %q", segment)
		}
	}
	if !githubOwnerPattern.MatchString(segments[0]) {
		return "", fmt.Errorf("projects: invalid github owner %q", segments[0])
	}
	return strings.Join(segments, "/"), nil
}

func validateBranch(branch Branch) error {
	if branch.Name == "" || strings.TrimSpace(branch.Name) != branch.Name {
		return fmt.Errorf("projects: branch name is required")
	}
	switch branch.Kind {
	case BranchLocal, BranchRemote:
		return nil
	default:
		return fmt.Errorf("projects: invalid branch kind %q", branch.Kind)
	}
}
