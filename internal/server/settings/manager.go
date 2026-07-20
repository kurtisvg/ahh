package settings

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

const (
	AuthenticationManaged AuthenticationMode = "managed"
	AuthenticationAmbient AuthenticationMode = "ambient"

	IdentityReady   IdentityStatus = "ready"
	IdentityInvalid IdentityStatus = "invalid"
)

// AuthenticationMode selects how Ahh authenticates Git operations.
type AuthenticationMode string

// IdentityStatus reports whether the managed private key is usable.
type IdentityStatus string

// Identity is the public representation of Ahh's managed SSH identity.
type Identity struct {
	Status      IdentityStatus `json:"status"`
	Algorithm   string         `json:"algorithm"`
	PublicKey   string         `json:"public_key,omitempty"`
	Fingerprint string         `json:"fingerprint,omitempty"`
}

// Settings is the complete public Settings representation.
type Settings struct {
	AuthenticationMode AuthenticationMode `json:"authentication_mode"`
	SSHIdentity        Identity           `json:"ssh_identity"`
}

type configuration struct {
	AuthenticationMode AuthenticationMode `json:"authentication_mode"`
}

// Manager securely persists installation Settings and the managed SSH keypair.
type Manager struct {
	mu       sync.Mutex
	dir      string
	settings Settings
}

// NewManager loads or initializes Settings beneath stateDir.
func NewManager(stateDir string) (*Manager, error) {
	if strings.TrimSpace(stateDir) == "" {
		return nil, fmt.Errorf("settings state directory is required")
	}

	m := &Manager{dir: filepath.Join(stateDir, "settings")}
	if err := m.ensureDirectory(); err != nil {
		return nil, err
	}
	if err := m.loadConfiguration(); err != nil {
		return nil, err
	}
	if err := m.loadOrCreateIdentity(); err != nil {
		return nil, err
	}

	return m, nil
}

// Get returns the current public Settings representation.
func (m *Manager) Get() Settings {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.settings
}

// SetAuthenticationMode persists and returns a new authentication mode.
func (m *Manager) SetAuthenticationMode(mode AuthenticationMode) (Settings, error) {
	if err := validateAuthenticationMode(mode); err != nil {
		return Settings{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.writeConfiguration(configuration{AuthenticationMode: mode}); err != nil {
		return Settings{}, err
	}
	m.settings.AuthenticationMode = mode

	return m.settings, nil
}

// Regenerate replaces the managed keypair when confirmFingerprint matches the
// currently displayed fingerprint.
func (m *Manager) Regenerate(confirmFingerprint string) (Settings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if confirmFingerprint == "" || confirmFingerprint != m.settings.SSHIdentity.Fingerprint {
		return Settings{}, ErrFingerprintConfirmation
	}

	identity, err := m.generateIdentity()
	if err != nil {
		return Settings{}, err
	}
	m.settings.SSHIdentity = identity

	return m.settings, nil
}

// GitEnvironment returns Git-specific environment entries. Managed mode
// selects only the installation key; background operations also disable
// credential prompts.
func (m *Manager) GitEnvironment(background bool) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	env := make([]string, 0, 3)
	if background {
		env = append(env, "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=Never")
	}
	if m.settings.AuthenticationMode == AuthenticationAmbient {
		return env, nil
	}
	if m.settings.SSHIdentity.Status != IdentityReady {
		return nil, ErrManagedIdentityInvalid
	}

	command := "ssh -i " + shellQuote(m.privateKeyPath()) + " -o IdentitiesOnly=yes"
	return append(env, "GIT_SSH_COMMAND="+command), nil
}

var (
	ErrFingerprintConfirmation = errors.New("settings: ssh fingerprint confirmation does not match")
	ErrManagedIdentityInvalid  = errors.New("settings: managed ssh identity is invalid")
)

func (m *Manager) loadConfiguration() error {
	data, err := os.ReadFile(m.configurationPath())
	if errors.Is(err, os.ErrNotExist) {
		m.settings.AuthenticationMode = AuthenticationManaged
		return m.writeConfiguration(configuration{AuthenticationMode: AuthenticationManaged})
	}
	if err != nil {
		return fmt.Errorf("read settings configuration: %w", err)
	}

	var cfg configuration
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return fmt.Errorf("decode settings configuration: %w", err)
	}
	if err := validateAuthenticationMode(cfg.AuthenticationMode); err != nil {
		return err
	}
	if err := os.Chmod(m.configurationPath(), 0o600); err != nil {
		return fmt.Errorf("secure settings configuration: %w", err)
	}
	m.settings.AuthenticationMode = cfg.AuthenticationMode

	return nil
}

func (m *Manager) loadOrCreateIdentity() error {
	privateData, err := os.ReadFile(m.privateKeyPath())
	if errors.Is(err, os.ErrNotExist) {
		identity, err := m.generateIdentity()
		if err != nil {
			return err
		}
		m.settings.SSHIdentity = identity
		return nil
	}
	if err != nil {
		return fmt.Errorf("read managed ssh identity: %w", err)
	}
	if err := os.Chmod(m.privateKeyPath(), 0o600); err != nil {
		return fmt.Errorf("secure managed ssh identity: %w", err)
	}

	raw, err := ssh.ParseRawPrivateKey(privateData)
	if err != nil {
		m.settings.SSHIdentity = m.invalidIdentityFromPublicFile()
		return nil
	}
	privateKey, ok := raw.(*ed25519.PrivateKey)
	if !ok {
		m.settings.SSHIdentity = m.invalidIdentityFromPublicFile()
		return nil
	}

	identity, publicData, err := publicIdentity(*privateKey)
	if err != nil {
		return err
	}
	storedPublic, readErr := os.ReadFile(m.publicKeyPath())
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("read managed ssh public identity: %w", readErr)
	}
	if readErr != nil || string(storedPublic) != string(publicData) {
		if err := atomicWrite(m.publicKeyPath(), publicData); err != nil {
			return fmt.Errorf("repair managed ssh public identity: %w", err)
		}
	} else if err := os.Chmod(m.publicKeyPath(), 0o600); err != nil {
		return fmt.Errorf("secure managed ssh public identity: %w", err)
	}
	m.settings.SSHIdentity = identity

	return nil
}

func (m *Manager) generateIdentity() (Identity, error) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Identity{}, fmt.Errorf("generate managed ssh identity: %w", err)
	}
	privateBlock, err := ssh.MarshalPrivateKey(privateKey, "ahh")
	if err != nil {
		return Identity{}, fmt.Errorf("serialize managed ssh identity: %w", err)
	}
	identity, publicData, err := publicIdentity(privateKey)
	if err != nil {
		return Identity{}, err
	}

	if err := atomicWrite(m.privateKeyPath(), pem.EncodeToMemory(privateBlock)); err != nil {
		return Identity{}, fmt.Errorf("write managed ssh identity: %w", err)
	}
	if err := atomicWrite(m.publicKeyPath(), publicData); err != nil {
		return Identity{}, fmt.Errorf("write managed ssh public identity: %w", err)
	}

	return identity, nil
}

func (m *Manager) invalidIdentityFromPublicFile() Identity {
	identity := Identity{Status: IdentityInvalid, Algorithm: ssh.KeyAlgoED25519}
	data, err := os.ReadFile(m.publicKeyPath())
	if err != nil {
		return identity
	}
	publicKey, _, _, _, err := ssh.ParseAuthorizedKey(data)
	if err != nil {
		return identity
	}
	identity.PublicKey = strings.TrimSpace(string(data))
	identity.Fingerprint = ssh.FingerprintSHA256(publicKey)
	return identity
}

func publicIdentity(privateKey ed25519.PrivateKey) (Identity, []byte, error) {
	publicKey, err := ssh.NewPublicKey(privateKey.Public())
	if err != nil {
		return Identity{}, nil, fmt.Errorf("derive managed ssh public identity: %w", err)
	}
	publicData := []byte(strings.TrimSpace(string(ssh.MarshalAuthorizedKey(publicKey))) + " ahh\n")
	return Identity{
		Status:      IdentityReady,
		Algorithm:   publicKey.Type(),
		PublicKey:   strings.TrimSpace(string(publicData)),
		Fingerprint: ssh.FingerprintSHA256(publicKey),
	}, publicData, nil
}

func validateAuthenticationMode(mode AuthenticationMode) error {
	switch mode {
	case AuthenticationManaged, AuthenticationAmbient:
		return nil
	default:
		return fmt.Errorf("settings: invalid authentication mode %q", mode)
	}
}

func (m *Manager) ensureDirectory() error {
	if err := os.MkdirAll(m.dir, 0o700); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}
	if err := os.Chmod(m.dir, 0o700); err != nil {
		return fmt.Errorf("secure settings directory: %w", err)
	}
	return nil
}

func (m *Manager) writeConfiguration(cfg configuration) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings configuration: %w", err)
	}
	data = append(data, '\n')
	if err := atomicWrite(m.configurationPath(), data); err != nil {
		return fmt.Errorf("write settings configuration: %w", err)
	}
	return nil
}

func atomicWrite(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func (m *Manager) configurationPath() string { return filepath.Join(m.dir, "settings.json") }
func (m *Manager) privateKeyPath() string    { return filepath.Join(m.dir, "ssh_identity") }
func (m *Manager) publicKeyPath() string     { return filepath.Join(m.dir, "ssh_identity.pub") }
