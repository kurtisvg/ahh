package projects

import "testing"

func TestNormalizeGitHubRepository(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  string
		wantError bool
	}{
		{name: "repository", input: "kurtisvg/ahh", expected: "kurtisvg/ahh"},
		{name: "optional git suffix", input: "kurtisvg/ahh.git", expected: "kurtisvg/ahh"},
		{name: "dots within repository", input: "owner/repo.name", expected: "owner/repo.name"},
		{name: "url", input: "https://github.com/owner/repo", wantError: true},
		{name: "ssh url", input: "git@github.com:owner/repo.git", wantError: true},
		{name: "host prefix", input: "github.com/owner", wantError: true},
		{name: "empty owner", input: "/repo", wantError: true},
		{name: "empty repository", input: "owner/", wantError: true},
		{name: "extra segment", input: "owner/group/repo", wantError: true},
		{name: "dot segment", input: "./repo", wantError: true},
		{name: "dot dot segment", input: "owner/..", wantError: true},
		{name: "leading whitespace", input: " owner/repo", wantError: true},
		{name: "embedded whitespace", input: "owner/re po", wantError: true},
		{name: "control character", input: "owner/repo\n", wantError: true},
		{name: "double git suffix", input: "owner/repo.git.git", expected: "owner/repo.git"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := NormalizeGitHubRepository(tt.input)
			if tt.wantError {
				if err == nil {
					t.Fatalf("NormalizeGitHubRepository(%q) error = nil, want error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeGitHubRepository(%q) error = %v", tt.input, err)
			}
			if actual != tt.expected {
				t.Fatalf("NormalizeGitHubRepository(%q) = %q, want %q", tt.input, actual, tt.expected)
			}
		})
	}
}
