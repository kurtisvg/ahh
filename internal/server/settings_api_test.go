package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kurtisvg/ahh/internal/server/settings"
)

func TestServerSettingsAPI(t *testing.T) {
	server := startTestServer(t)
	defer shutdownTestServer(t, server)
	client := &http.Client{Timeout: 2 * time.Second}

	resp, err := client.Get("http://" + server.Addr + "/api/settings")
	if err != nil {
		t.Fatalf("GET /api/settings: %v", err)
	}
	assertStatus(t, resp, http.StatusOK)
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read Settings response: %v", err)
	}
	if bytes.Contains(body, []byte("PRIVATE KEY")) {
		t.Fatal("GET /api/settings exposed private key material")
	}
	var initial settings.Settings
	if err := json.Unmarshal(body, &initial); err != nil {
		t.Fatalf("decode Settings response: %v", err)
	}
	if initial.AuthenticationMode != settings.AuthenticationManaged || initial.SSHIdentity.Status != settings.IdentityReady {
		t.Fatalf("initial Settings = %+v, want managed ready identity", initial)
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPatch, "http://"+server.Addr+"/api/settings", strings.NewReader(`{"authentication_mode":"ambient"}`))
	if err != nil {
		t.Fatalf("new PATCH request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("PATCH /api/settings: %v", err)
	}
	assertStatus(t, resp, http.StatusOK)
	var ambient settings.Settings
	decodeJSON(t, resp, &ambient)
	resp.Body.Close()
	if ambient.AuthenticationMode != settings.AuthenticationAmbient || ambient.SSHIdentity != initial.SSHIdentity {
		t.Fatalf("updated Settings = %+v, want ambient with unchanged identity", ambient)
	}

	for _, body := range []string{
		`{"authentication_mode":"invalid"}`,
		`{"authentication_mode":"managed","unknown":true}`,
		`{}`,
	} {
		req, err = http.NewRequestWithContext(t.Context(), http.MethodPatch, "http://"+server.Addr+"/api/settings", strings.NewReader(body))
		if err != nil {
			t.Fatalf("new invalid PATCH request: %v", err)
		}
		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("invalid PATCH /api/settings: %v", err)
		}
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	}

	resp, err = client.Post("http://"+server.Addr+"/api/settings/ssh-identity/regenerate", "application/json", strings.NewReader(`{"confirm_fingerprint":"SHA256:stale"}`))
	if err != nil {
		t.Fatalf("POST stale regeneration: %v", err)
	}
	assertStatus(t, resp, http.StatusConflict)
	resp.Body.Close()

	regenerateBody, err := json.Marshal(regenerateIdentityRequest{ConfirmFingerprint: initial.SSHIdentity.Fingerprint})
	if err != nil {
		t.Fatalf("marshal regeneration request: %v", err)
	}
	resp, err = client.Post("http://"+server.Addr+"/api/settings/ssh-identity/regenerate", "application/json", bytes.NewReader(regenerateBody))
	if err != nil {
		t.Fatalf("POST regeneration: %v", err)
	}
	assertStatus(t, resp, http.StatusOK)
	var regenerated settings.Settings
	decodeJSON(t, resp, &regenerated)
	resp.Body.Close()
	if regenerated.SSHIdentity.Fingerprint == initial.SSHIdentity.Fingerprint {
		t.Fatalf("regenerated fingerprint = %q, want a new key", regenerated.SSHIdentity.Fingerprint)
	}
}
