package server

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/kurtisvg/ahh/internal/store"
	"github.com/kurtisvg/ahh/internal/store/sqlite"
)

// Config selects the workspace, HTTP listener, and metadata backend.
type Config struct {
	Address   string
	Port      int
	Workspace string       // Required workspace directory; relative to the startup directory.
	Store     store.Config // Relative SQLite paths resolve within Workspace.

	// BasePath is the public URL prefix, stripped by the proxy. Empty means root.
	BasePath string
}

func normalizeBasePath(value string) (string, error) {
	if value == "" || value == "/" {
		return "/", nil
	}
	u, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("parse base path: %w", err)
	}
	absPath := strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//")
	plainPath := !strings.ContainsAny(value, "\\?#%")
	cleanPath := path.Clean(u.Path) == strings.TrimSuffix(u.Path, "/")
	if !absPath || !plainPath || !cleanPath {
		return "", fmt.Errorf("invalid base path %q: use a path such as / or /proxy/40443/", value)
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/"
	return u.EscapedPath(), nil
}

func openStore(ctx context.Context, config store.Config) (store.Store, error) {
	switch config.Backend {
	case store.SQLite:
		if config.Path == "" {
			return nil, errors.New("sqlite database path is required")
		}
		return sqlite.Open(ctx, config.Path)
	default:
		return nil, fmt.Errorf("unsupported store backend %q", config.Backend)
	}
}
