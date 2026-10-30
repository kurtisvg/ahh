package server

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kurtisvg/ahh/internal/store"
)

func TestWorkspaceLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace")
	lock, err := lockWorkspace(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	child := func() error {
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestWorkspaceLockChild$")
		cmd.Env = append(os.Environ(), "AHH_TEST_WORKSPACE="+alias)
		return cmd.Run()
	}
	if err := child(); err == nil {
		t.Fatal("second process acquired workspace through alias")
	}
	info, err := os.Stat(filepath.Join(path, ".ahh.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	if err := child(); err != nil {
		t.Fatalf("closed workspace still locked: %v", err)
	}
	after, err := os.Stat(filepath.Join(path, ".ahh.lock"))
	if err != nil || !os.SameFile(info, after) {
		t.Fatalf("lock file replaced: %v", err)
	}
}

func TestWorkspaceLockChild(t *testing.T) {
	path := os.Getenv("AHH_TEST_WORKSPACE")
	if path == "" {
		t.Skip("subprocess helper")
	}
	lock, err := lockWorkspace(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if os.Getenv("AHH_TEST_HOLD_LOCK") == "1" {
		fmt.Println("locked")
		time.Sleep(time.Hour) // The parent kills this process after reading the signal.
	}
}

func TestWorkspaceLockReleasedAfterKill(t *testing.T) {
	path := t.TempDir()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestWorkspaceLockChild$")
	cmd.Env = append(os.Environ(), "AHH_TEST_WORKSPACE="+path, "AHH_TEST_HOLD_LOCK=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || line != "locked\n" {
		t.Fatalf("child lock acquisition: %q, %v", line, err)
	}
	if lock, err := lockWorkspace(path); err == nil {
		_ = lock.Close()
		t.Fatal("acquired lock held by child")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	lock, err := lockWorkspace(path)
	if err != nil {
		t.Fatalf("killed process retained lock: %v", err)
	}
	_ = lock.Close()
}

func TestWorkspaceLockRejectsSymlink(t *testing.T) {
	path := t.TempDir()
	target := filepath.Join(t.TempDir(), "unrelated")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(path, ".ahh.lock")); err != nil {
		t.Fatal(err)
	}
	if lock, err := lockWorkspace(path); err == nil {
		_ = lock.Close()
		t.Fatal("followed lock symlink")
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "keep" {
		t.Fatalf("changed lock target: %q, %v", data, err)
	}
}

func TestServerOwnsWorkspace(t *testing.T) {
	t.Chdir(t.TempDir())
	config := Config{
		Address: "127.0.0.1", Workspace: "./workspace",
		Store: store.Config{Backend: store.SQLite, Path: "ahh.db"},
	}
	s, err := Start(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Shutdown(t.Context()); _ = s.Wait() })
	want, _ := filepath.Abs("workspace")
	if s.Workspace != want {
		t.Fatalf("workspace = %q, want %q", s.Workspace, want)
	}
	if _, err := os.Stat(filepath.Join(want, "ahh.db")); err != nil {
		t.Fatal(err)
	}
	config.Store.Path = "second.db"
	if second, err := Start(t.Context(), config); err == nil {
		_ = second.Shutdown(t.Context())
		_ = second.Wait()
		t.Fatal("second server acquired workspace")
	}
	if _, err := os.Stat(filepath.Join(want, "second.db")); !os.IsNotExist(err) {
		t.Fatalf("opened storage before acquiring lock: %v", err)
	}
	_ = s.Shutdown(t.Context())
	if err := s.Wait(); err != nil {
		t.Fatal(err)
	}
	lock, err := lockWorkspace(want)
	if err != nil {
		t.Fatalf("server retained lock after Wait: %v", err)
	}
	_ = lock.Close()
}

func TestStoreOpenFailureReleasesWorkspace(t *testing.T) {
	path := t.TempDir()
	_, err := Start(t.Context(), Config{Workspace: path})
	if err == nil {
		t.Fatal("started without a storage backend")
	}
	lock, err := lockWorkspace(path)
	if err != nil {
		t.Fatalf("store failure retained lock: %v", err)
	}
	_ = lock.Close()
}
