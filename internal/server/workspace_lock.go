package server

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// lockWorkspace creates the workspace and excludes other local servers.
// Close the returned file only after all workspace users have stopped. Never
// delete it: another server could then lock a different file at the same path.
func lockWorkspace(path string) (*os.File, error) {
	if path == "" {
		return nil, errors.New("workspace path is required")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(path, ".ahh.lock"), os.O_CREATE|os.O_RDWR|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open workspace lock: %w", err)
	}
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return nil, errors.Join(fmt.Errorf("workspace is in use or cannot be locked: %w", err), lock.Close())
	}
	return lock, nil
}
