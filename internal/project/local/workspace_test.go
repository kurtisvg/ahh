package local

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRootLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	child := func() error {
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestRootLockChild$")
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
	if err := d.Close(); err != nil {
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

func TestRootLockChild(t *testing.T) {
	path := os.Getenv("AHH_TEST_WORKSPACE")
	if path == "" {
		t.Skip("subprocess helper")
	}
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRootFailureReleasesLock(t *testing.T) {
	path := t.TempDir()
	if err := os.WriteFile(filepath.Join(path, "projects"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if d, err := Open(path); err == nil {
		_ = d.Close()
		t.Fatal("accepted a file as projects directory")
	}
	if err := os.Rename(filepath.Join(path, "projects"), filepath.Join(path, "unrelated")); err != nil {
		t.Fatal(err)
	}
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if _, err := os.Stat(filepath.Join(path, "unrelated")); err != nil {
		t.Fatal("workspace open changed existing contents")
	}
}

func TestRelativeRoot(t *testing.T) {
	t.Chdir(t.TempDir())
	d, err := Open("./workspace")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	want, _ := filepath.Abs("workspace")
	if d.Path() != want {
		t.Fatalf("root = %q, want %q", d.Path(), want)
	}
}
