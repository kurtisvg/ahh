package settings

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestManagerGeneratesAndReloadsIdentity(t *testing.T) {
	stateDir := t.TempDir()
	manager, err := NewManager(stateDir)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	initial := manager.Get()
	if initial.AuthenticationMode != AuthenticationManaged {
		t.Fatalf("authentication mode = %q, want managed", initial.AuthenticationMode)
	}
	if initial.SSHIdentity.Status != IdentityReady || initial.SSHIdentity.Algorithm != ssh.KeyAlgoED25519 {
		t.Fatalf("identity = %+v, want ready ed25519", initial.SSHIdentity)
	}
	if !strings.HasSuffix(initial.SSHIdentity.PublicKey, " ahh") {
		t.Fatalf("public key = %q, want ahh comment", initial.SSHIdentity.PublicKey)
	}
	if !strings.HasPrefix(initial.SSHIdentity.Fingerprint, "SHA256:") {
		t.Fatalf("fingerprint = %q, want SHA256 fingerprint", initial.SSHIdentity.Fingerprint)
	}

	privatePath := filepath.Join(stateDir, "settings", "ssh_identity")
	privateBefore, err := os.ReadFile(privatePath)
	if err != nil {
		t.Fatalf("read private key: %v", err)
	}
	if bytes.Contains(privateBefore, []byte(initial.SSHIdentity.PublicKey)) {
		t.Fatal("private key file unexpectedly contains authorized public key representation")
	}

	reloaded, err := NewManager(stateDir)
	if err != nil {
		t.Fatalf("reload NewManager() error = %v", err)
	}
	if got := reloaded.Get(); got != initial {
		t.Fatalf("reloaded Settings = %+v, want %+v", got, initial)
	}
	privateAfter, err := os.ReadFile(privatePath)
	if err != nil {
		t.Fatalf("read reloaded private key: %v", err)
	}
	if !bytes.Equal(privateAfter, privateBefore) {
		t.Fatal("reload replaced the existing private key")
	}
}

func TestManagerSecuresFilesAndRepairsPublicKey(t *testing.T) {
	stateDir := t.TempDir()
	manager, err := NewManager(stateDir)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	wantPublic := manager.Get().SSHIdentity.PublicKey + "\n"

	settingsDir := filepath.Join(stateDir, "settings")
	publicPath := filepath.Join(settingsDir, "ssh_identity.pub")
	if err := os.WriteFile(publicPath, []byte("mismatched\n"), 0o666); err != nil {
		t.Fatalf("replace public key: %v", err)
	}
	if _, err := NewManager(stateDir); err != nil {
		t.Fatalf("repair NewManager() error = %v", err)
	}
	repaired, err := os.ReadFile(publicPath)
	if err != nil {
		t.Fatalf("read repaired public key: %v", err)
	}
	if string(repaired) != wantPublic {
		t.Fatalf("repaired public key = %q, want %q", repaired, wantPublic)
	}

	assertMode(t, settingsDir, 0o700)
	for _, name := range []string{"settings.json", "ssh_identity", "ssh_identity.pub"} {
		assertMode(t, filepath.Join(settingsDir, name), 0o600)
	}
}

func TestManagerPreservesCorruptPrivateKeyAndAllowsConfirmedRegeneration(t *testing.T) {
	stateDir := t.TempDir()
	manager, err := NewManager(stateDir)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	old := manager.Get().SSHIdentity
	privatePath := filepath.Join(stateDir, "settings", "ssh_identity")
	corrupt := []byte("not an openssh private key\n")
	if err := os.WriteFile(privatePath, corrupt, 0o600); err != nil {
		t.Fatalf("corrupt private key: %v", err)
	}

	reloaded, err := NewManager(stateDir)
	if err != nil {
		t.Fatalf("reload corrupt NewManager() error = %v", err)
	}
	invalid := reloaded.Get().SSHIdentity
	if invalid.Status != IdentityInvalid || invalid.Fingerprint != old.Fingerprint {
		t.Fatalf("invalid identity = %+v, want invalid with displayed old fingerprint", invalid)
	}
	gotCorrupt, err := os.ReadFile(privatePath)
	if err != nil {
		t.Fatalf("read corrupt private key: %v", err)
	}
	if !bytes.Equal(gotCorrupt, corrupt) {
		t.Fatal("corrupt private key was silently replaced")
	}
	if _, err := reloaded.GitEnvironment(true); !errors.Is(err, ErrManagedIdentityInvalid) {
		t.Fatalf("GitEnvironment() error = %v, want ErrManagedIdentityInvalid", err)
	}
	if _, err := reloaded.Regenerate("SHA256:stale"); !errors.Is(err, ErrFingerprintConfirmation) {
		t.Fatalf("Regenerate(stale) error = %v, want confirmation error", err)
	}
	regenerated, err := reloaded.Regenerate(invalid.Fingerprint)
	if err != nil {
		t.Fatalf("Regenerate() error = %v", err)
	}
	if regenerated.SSHIdentity.Status != IdentityReady || regenerated.SSHIdentity.Fingerprint == old.Fingerprint {
		t.Fatalf("regenerated identity = %+v, want new ready identity", regenerated.SSHIdentity)
	}
}

func TestManagerAuthenticationModeAndGitEnvironment(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state dir's value")
	manager, err := NewManager(stateDir)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	env, err := manager.GitEnvironment(true)
	if err != nil {
		t.Fatalf("GitEnvironment(managed) error = %v", err)
	}
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "GIT_TERMINAL_PROMPT=0") || !strings.Contains(joined, "GCM_INTERACTIVE=Never") {
		t.Fatalf("background environment = %q, want prompts disabled", joined)
	}
	wantQuotedPath := "'" + strings.ReplaceAll(filepath.Join(stateDir, "settings", "ssh_identity"), "'", "'\\''") + "'"
	if !strings.Contains(joined, "ssh -i "+wantQuotedPath+" -o IdentitiesOnly=yes") {
		t.Fatalf("managed environment = %q, want safely quoted identity path %q", joined, wantQuotedPath)
	}

	updated, err := manager.SetAuthenticationMode(AuthenticationAmbient)
	if err != nil {
		t.Fatalf("SetAuthenticationMode(ambient) error = %v", err)
	}
	if updated.AuthenticationMode != AuthenticationAmbient {
		t.Fatalf("updated mode = %q, want ambient", updated.AuthenticationMode)
	}
	ambient, err := manager.GitEnvironment(false)
	if err != nil {
		t.Fatalf("GitEnvironment(ambient) error = %v", err)
	}
	if len(ambient) != 0 {
		t.Fatalf("interactive ambient environment = %q, want untouched", ambient)
	}
	if _, err := NewManager(stateDir); err != nil {
		t.Fatalf("reload ambient manager: %v", err)
	}
	if _, err := manager.SetAuthenticationMode("invalid"); err == nil {
		t.Fatal("SetAuthenticationMode(invalid) error = nil, want error")
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode %q = %o, want %o", path, got, want)
	}
}
