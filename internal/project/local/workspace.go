// Package local manages Project repositories on a local filesystem.
package local

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"
)

// Workspace owns access to one workspace root. Close after all users and
// managed writers have stopped. Independent Project operations may run concurrently.
type Workspace struct {
	root *os.Root
	lock *os.File
	once sync.Once // Prevents repeated or concurrent Close calls from closing handles twice.
	err  error
}

// Open creates or reopens a workspace and excludes other local servers.
// The lock file stays in place; its existence does not indicate a held lock.
func Open(path string) (*Workspace, error) {
	if path == "" {
		return nil, errors.New("workspace path is empty")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	// Resolve workspace aliases before opening the root so diagnostics use one path.
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	// Keep the lock in this workspace and reject a symlink at the lock filename.
	lock, err := root.OpenFile(".ahh.lock", os.O_CREATE|os.O_RDWR|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("open workspace lock: %w", err)
	}
	// Fail immediately if another server holds the lock. Keep the file open until
	// Close; deleting it could let another server lock a different file at this path.
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = lock.Close()
		_ = root.Close()
		return nil, fmt.Errorf("workspace is in use or cannot be locked: %w", err)
	}
	// Initialization failures must release ownership so a later attempt can retry.
	w := &Workspace{root: root, lock: lock}
	if err := root.MkdirAll("projects", 0o700); err != nil {
		_ = w.Close()
		return nil, fmt.Errorf("create projects directory: %w", err)
	}
	return w, nil
}

// Path is the canonical root used for startup diagnostics and metadata placement.
func (w *Workspace) Path() string { return w.root.Name() }

// Close releases the root and lock once all users have finished.
// Repeated or concurrent calls wait for the same cleanup and return its result.
func (w *Workspace) Close() error {
	w.once.Do(func() { w.err = errors.Join(w.root.Close(), w.lock.Close()) })
	return w.err
}
