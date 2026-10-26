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
				Address: "127.0.0.1", Store: tt.config,
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
