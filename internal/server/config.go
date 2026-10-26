package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/kurtisvg/ahh/internal/store"
	"github.com/kurtisvg/ahh/internal/store/sqlite"
)

// Config selects the HTTP listener and metadata backend.
type Config struct {
	Address string
	Port    int
	Store   store.Config
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
