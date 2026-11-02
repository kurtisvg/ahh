// Package sqlite implements metadata storage with SQLite.
package sqlite

import (
	"context"
	"embed"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	"golang.org/x/sys/unix"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Store owns the database pool. Close after application requests have finished.
type Store struct{ db *sqlx.DB }

// Open requires exclusive workspace ownership before migration.
func Open(ctx context.Context, path string) (*Store, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve metadata path: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open metadata file: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: path}
	query := url.Values{"_pragma": {"busy_timeout(5000)", "foreign_keys(1)"}}
	u.RawQuery = query.Encode()
	db, err := sqlx.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate metadata: %w", err)
	}
	return s, nil
}

func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var version int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	const latestVersion = 2
	if version < 0 || version > latestVersion {
		return fmt.Errorf("unsupported schema version %d", version)
	}
	for next := version + 1; next <= latestVersion; next++ {
		path := fmt.Sprintf("migrations/%03d.sql", next)
		script, err := migrations.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(script)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Close() error { return s.db.Close() }
