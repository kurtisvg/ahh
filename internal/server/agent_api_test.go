package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurtisvg/ahh/internal/server/agents"
)

func TestServerAgentsAPI(t *testing.T) {
	manager, err := agents.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("agents.NewManager() error = %v", err)
	}
	server := startTestServer(t, WithAgentManager(manager))
	defer shutdownTestServer(t, server)
	client := &http.Client{Timeout: 2 * time.Second}

	resp, err := client.Get("http://" + server.Addr + "/api/agents")
	if err != nil {
		t.Fatalf("GET /api/agents: %v", err)
	}
	assertStatus(t, resp, http.StatusOK)
	var initial agentsResponse
	decodeJSON(t, resp, &initial)
	_ = resp.Body.Close()
	if len(initial.Agents) != 1 || initial.Agents[0].Name != agents.DefaultAgentName {
		t.Fatalf("initial Agents = %#v, want default Agent", initial.Agents)
	}
	defaultID := initial.Agents[0].ID

	created := createAgentViaAPI(t, client, server, "Build Agent", agents.ClaudeCodeHarness)
	if id, parseErr := uuid.Parse(created.ID); parseErr != nil || id.Version() != 4 || created.ID == defaultID {
		t.Fatalf("created Agent ID = %q, want a distinct UUID v4", created.ID)
	}

	resp = doJSONRequest(t, client, http.MethodPost, "http://"+server.Addr+"/api/agents", map[string]string{
		"name":    "build agent",
		"harness": agents.ClaudeCodeHarness,
	})
	assertStatus(t, resp, http.StatusConflict)
	assertAPIError(t, resp, "agent name already exists")

	resp = doJSONRequest(t, client, http.MethodPost, "http://"+server.Addr+"/api/agents", map[string]string{
		"name":    "Codex",
		"harness": "codex",
	})
	assertStatus(t, resp, http.StatusBadRequest)
	assertAPIError(t, resp, "unsupported agent harness")

	resp = doJSONRequest(t, client, http.MethodPatch, "http://"+server.Addr+"/api/agents/"+created.ID, map[string]string{
		"name": "Release Agent",
	})
	assertStatus(t, resp, http.StatusOK)
	var renamed agents.Config
	decodeJSON(t, resp, &renamed)
	_ = resp.Body.Close()
	if renamed.ID != created.ID || renamed.Name != "Release Agent" || renamed.Harness != created.Harness {
		t.Fatalf("renamed Agent = %#v, want immutable ID/harness and updated name", renamed)
	}

	resp = doJSONRequest(t, client, http.MethodPatch, "http://"+server.Addr+"/api/agents/missing", map[string]string{
		"name": "Missing",
	})
	assertStatus(t, resp, http.StatusNotFound)
	assertAPIError(t, resp, "agent not found")

	resp = doJSONRequest(t, client, http.MethodPatch, "http://"+server.Addr+"/api/agents/"+created.ID, map[string]string{
		"name": agents.DefaultAgentName,
	})
	assertStatus(t, resp, http.StatusConflict)
	assertAPIError(t, resp, "agent name already exists")

	resp = doJSONRequest(t, client, http.MethodPatch, "http://"+server.Addr+"/api/agents/"+created.ID, map[string]string{
		"name":    "Ignored mutation",
		"harness": "codex",
	})
	assertStatus(t, resp, http.StatusBadRequest)
	assertAPIError(t, resp, "invalid agent request")

	resp, err = client.Get("http://" + server.Addr + "/api/agents")
	if err != nil {
		t.Fatalf("GET /api/agents after changes: %v", err)
	}
	assertStatus(t, resp, http.StatusOK)
	var listed agentsResponse
	decodeJSON(t, resp, &listed)
	_ = resp.Body.Close()
	if len(listed.Agents) != 2 || listed.Agents[0].Name != agents.DefaultAgentName || listed.Agents[1].Name != "Release Agent" {
		t.Fatalf("listed Agents = %#v, want name-sorted Agents", listed.Agents)
	}
}

func TestAgentAPIRejectsInvalidRequestsAndDelete(t *testing.T) {
	server := startTestServer(t)
	defer shutdownTestServer(t, server)
	client := &http.Client{Timeout: 2 * time.Second}

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		status int
	}{
		{name: "malformed create", method: http.MethodPost, path: "/api/agents", body: "{", status: http.StatusBadRequest},
		{name: "blank name", method: http.MethodPost, path: "/api/agents", body: `{"name":" ","harness":"claude-code"}`, status: http.StatusBadRequest},
		{name: "missing harness", method: http.MethodPost, path: "/api/agents", body: `{"name":"Agent"}`, status: http.StatusBadRequest},
		{name: "blank rename", method: http.MethodPatch, path: "/api/agents/claude-code", body: `{"name":" "}`, status: http.StatusBadRequest},
		{name: "delete unsupported", method: http.MethodDelete, path: "/api/agents/claude-code", status: http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), tt.method, "http://"+server.Addr+tt.path, strings.NewReader(tt.body))
			if err != nil {
				t.Fatalf("NewRequest() error = %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Do() error = %v", err)
			}
			defer resp.Body.Close()
			assertStatus(t, resp, tt.status)
		})
	}
}

func createAgentViaAPI(t *testing.T, client *http.Client, server *Server, name, harness string) agents.Config {
	t.Helper()
	resp := doJSONRequest(t, client, http.MethodPost, "http://"+server.Addr+"/api/agents", map[string]string{
		"name": name, "harness": harness,
	})
	assertStatus(t, resp, http.StatusCreated)
	defer resp.Body.Close()
	var agent agents.Config
	decodeJSON(t, resp, &agent)
	return agent
}

func doJSONRequest(t *testing.T, client *http.Client, method, url string, value any) *http.Response {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, url, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

func assertAPIError(t *testing.T, resp *http.Response, want string) {
	t.Helper()
	defer resp.Body.Close()
	var apiErr errorResponse
	decodeJSON(t, resp, &apiErr)
	if apiErr.Error != want {
		t.Fatalf("API error = %q, want %q (status %d)", apiErr.Error, want, resp.StatusCode)
	}
}
