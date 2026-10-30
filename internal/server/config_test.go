package server

import (
	"strings"
	"testing"

	"github.com/kurtisvg/ahh/internal/store"
)

func TestStartRejectsInvalidStorage(t *testing.T) {
	for _, tt := range []struct {
		name   string
		config store.Config
		want   string
	}{
		{"missing backend", store.Config{}, "unsupported store backend"},
		{"missing path", store.Config{Backend: store.SQLite}, "sqlite database path is required"},
		{"directory path", store.Config{Backend: store.SQLite, Path: t.TempDir()}, "open metadata file"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Start(t.Context(), Config{
				Address: "127.0.0.1", Workspace: t.TempDir(), Store: tt.config,
			})
			if s != nil {
				_ = s.Shutdown(t.Context())
				_ = s.Wait()
				t.Fatal("started server with invalid storage")
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("startup error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestNormalizeBasePath(t *testing.T) {
	for input, want := range map[string]string{
		"": "/", "/": "/", "/proxy/40443": "/proxy/40443/",
		"/proxy/40443/": "/proxy/40443/", "/my project": "/my%20project/",
	} {
		t.Run(input, func(t *testing.T) {
			got, err := normalizeBasePath(input)
			if err != nil || got != want {
				t.Fatalf("normalizeBasePath(%q) = %q, %v; want %q", input, got, err, want)
			}
		})
	}
	for _, input := range []string{
		"relative/path", "https://example.com/app/", "//example.com/",
		"/app?query", "/app#fragment", "/a/../b", "/a/./b", "/a//b",
		"/a\\b", "/%2e%2e/", "/app\n",
	} {
		t.Run(input, func(t *testing.T) {
			if _, err := normalizeBasePath(input); err == nil {
				t.Fatalf("accepted invalid base path %q", input)
			}
		})
	}
}
