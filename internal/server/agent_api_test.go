package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kurtisvg/ahh/internal/harness"
	"github.com/kurtisvg/ahh/internal/server/agents"
)

type agentAPITestRequest struct {
	method string
	path   string
	body   string
}

func TestListAgents(t *testing.T) {
	t.Parallel()

	manager, handler := newTestAgentAPI(t)
	for _, name := range []string{"Zulu", "Alpha"} {
		if _, err := manager.Create(name, harness.TypeClaudeCode); err != nil {
			t.Fatalf("Create(%q) error = %v", name, err)
		}
	}

	recorder := serveAgentAPI(t, handler, agentAPITestRequest{
		method: http.MethodGet,
		path:   "/api/agents",
	})
	assertAgentAPIStatus(t, recorder, http.StatusOK)

	var response agentsResponse
	decodeAgentAPIJSON(t, recorder, &response)
	gotNames := make([]string, 0, len(response.Agents))
	for _, agent := range response.Agents {
		gotNames = append(gotNames, agent.Name)
	}
	wantNames := []string{"Alpha", agents.DefaultAgentName, "Zulu"}
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("Agent names = %q, want %q", gotNames, wantNames)
	}
}

func TestCreateAgent(t *testing.T) {
	t.Parallel()

	manager, handler := newTestAgentAPI(t)
	recorder := serveAgentAPI(t, handler, agentAPITestRequest{
		method: http.MethodPost,
		path:   "/api/agents",
		body: marshalAgentAPIJSON(t, map[string]string{
			"name":    "  Build Agent  ",
			"harness": "  claude-code  ",
		}),
	})
	assertAgentAPIStatus(t, recorder, http.StatusCreated)

	var created agents.Config
	decodeAgentAPIJSON(t, recorder, &created)
	id, err := uuid.Parse(created.ID)
	if err != nil || id.Version() != 4 {
		t.Fatalf("created Agent ID = %q, want UUID v4", created.ID)
	}
	if created.Name != "Build Agent" || created.Harness != harness.TypeClaudeCode {
		t.Fatalf("created Agent = %#v, want normalized name and Claude Code harness", created)
	}
	persisted, ok := manager.Get(created.ID)
	if !ok || persisted != created {
		t.Fatalf("persisted Agent = %#v, %t, want %#v, true", persisted, ok, created)
	}
}

func TestCreateAgentRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantError  string
	}{
		{
			name:       "malformed request",
			body:       "{",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid agent request",
		},
		{
			name:       "blank name",
			body:       `{"name":" ","harness":"claude-code"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "agent name is required",
		},
		{
			name:       "missing harness",
			body:       `{"name":"Agent"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "agent harness is required",
		},
		{
			name:       "unsupported harness",
			body:       `{"name":"Codex","harness":"codex"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "unsupported agent harness",
		},
		{
			name:       "duplicate name",
			body:       `{"name":"DEFAULT","harness":"claude-code"}`,
			wantStatus: http.StatusConflict,
			wantError:  "agent name already exists",
		},
		{
			name:       "unknown field",
			body:       `{"name":"Agent","harness":"claude-code","extra":true}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid agent request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			manager, handler := newTestAgentAPI(t)
			recorder := serveAgentAPI(t, handler, agentAPITestRequest{
				method: http.MethodPost,
				path:   "/api/agents",
				body:   tt.body,
			})
			assertAgentAPIError(t, recorder, tt.wantStatus, tt.wantError)
			if got := manager.List(); len(got) != 1 || got[0].Name != agents.DefaultAgentName {
				t.Fatalf("Agents after rejected create = %#v, want only default Agent", got)
			}
		})
	}
}

func TestUpdateAgent(t *testing.T) {
	t.Parallel()

	manager, handler := newTestAgentAPI(t)
	created, err := manager.Create("Build Agent", harness.TypeClaudeCode)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	recorder := serveAgentAPI(t, handler, agentAPITestRequest{
		method: http.MethodPatch,
		path:   "/api/agents/" + created.ID,
		body:   `{"name":"  Release Agent  "}`,
	})
	assertAgentAPIStatus(t, recorder, http.StatusOK)

	var updated agents.Config
	decodeAgentAPIJSON(t, recorder, &updated)
	if updated.ID != created.ID || updated.Name != "Release Agent" || updated.Harness != created.Harness {
		t.Fatalf("updated Agent = %#v, want immutable ID/harness and normalized name", updated)
	}
	persisted, ok := manager.Get(created.ID)
	if !ok || persisted != updated {
		t.Fatalf("persisted Agent = %#v, %t, want %#v, true", persisted, ok, updated)
	}
}

func TestUpdateAgentRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		missing    bool
		wantStatus int
		wantError  string
	}{
		{
			name:       "malformed request",
			body:       "{",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid agent request",
		},
		{
			name:       "blank name",
			body:       `{"name":" "}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "agent name is required",
		},
		{
			name:       "missing agent",
			body:       `{"name":"Missing"}`,
			missing:    true,
			wantStatus: http.StatusNotFound,
			wantError:  "agent not found",
		},
		{
			name:       "duplicate name",
			body:       `{"name":"default"}`,
			wantStatus: http.StatusConflict,
			wantError:  "agent name already exists",
		},
		{
			name:       "immutable field",
			body:       `{"name":"Ignored mutation","harness":"codex"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid agent request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			manager, handler := newTestAgentAPI(t)
			created, err := manager.Create("Build Agent", harness.TypeClaudeCode)
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			targetID := created.ID
			if tt.missing {
				targetID = uuid.NewString()
			}

			recorder := serveAgentAPI(t, handler, agentAPITestRequest{
				method: http.MethodPatch,
				path:   "/api/agents/" + targetID,
				body:   tt.body,
			})
			assertAgentAPIError(t, recorder, tt.wantStatus, tt.wantError)
			persisted, ok := manager.Get(created.ID)
			if !ok || persisted != created {
				t.Fatalf("Agent after rejected update = %#v, %t, want %#v, true", persisted, ok, created)
			}
		})
	}
}

func TestDeleteAgentIsUnsupported(t *testing.T) {
	t.Parallel()

	manager, handler := newTestAgentAPI(t)
	defaultID := manager.List()[0].ID
	recorder := serveAgentAPI(t, handler, agentAPITestRequest{
		method: http.MethodDelete,
		path:   "/api/agents/" + defaultID,
	})
	assertAgentAPIStatus(t, recorder, http.StatusMethodNotAllowed)
	if _, ok := manager.Get(defaultID); !ok {
		t.Fatal("default Agent was removed by unsupported delete")
	}
}

func newTestAgentAPI(t *testing.T) (*agents.Manager, http.Handler) {
	t.Helper()

	manager, err := agents.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("agents.NewManager() error = %v", err)
	}
	server := &Server{agents: manager}
	handler := http.StripPrefix("/api", server.serveAPI())

	return manager, handler
}

func serveAgentAPI(
	t *testing.T,
	handler http.Handler,
	request agentAPITestRequest,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(request.method, request.path, strings.NewReader(request.body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	return recorder
}

func marshalAgentAPIJSON(t *testing.T, value any) string {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	return string(data)
}

func assertAgentAPIStatus(t *testing.T, recorder *httptest.ResponseRecorder, want int) {
	t.Helper()

	if recorder.Code != want {
		t.Fatalf("status = %d, want %d, body = %q", recorder.Code, want, recorder.Body.String())
	}
}

func assertAgentAPIError(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
	wantStatus int,
	wantError string,
) {
	t.Helper()
	assertAgentAPIStatus(t, recorder, wantStatus)

	var response errorResponse
	decodeAgentAPIJSON(t, recorder, &response)
	if response.Error != wantError {
		t.Fatalf("API error = %q, want %q", response.Error, wantError)
	}
}

func decodeAgentAPIJSON(t *testing.T, recorder *httptest.ResponseRecorder, value any) {
	t.Helper()

	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if err := json.NewDecoder(recorder.Body).Decode(value); err != nil {
		t.Fatalf("decode JSON response: %v", err)
	}
}
