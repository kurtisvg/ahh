package store_test

import (
	"testing"

	"github.com/kurtisvg/ahh/internal/store"
)

func TestParseBackend(t *testing.T) {
	backend, err := store.ParseBackend("sqlite")
	if err != nil || backend != store.SQLite || backend.String() != "sqlite" {
		t.Fatalf("parse sqlite = %v, %v", backend, err)
	}
	for _, value := range []string{"", "unknown", "SQLITE", " sqlite "} {
		t.Run(value, func(t *testing.T) {
			backend, err := store.ParseBackend(value)
			if err == nil || backend != (store.Backend{}) {
				t.Fatalf("parse %q = %v, %v; want unset backend and error", value, backend, err)
			}
		})
	}
}
