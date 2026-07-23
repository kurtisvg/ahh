package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/kurtisvg/ahh/internal/harness"
	"github.com/kurtisvg/ahh/internal/server/agents"
	"github.com/kurtisvg/ahh/internal/server/conversations"
	"github.com/kurtisvg/ahh/internal/wrapper"
)

func TestServerHTTP(t *testing.T) {
	tests := []struct {
		name             string
		method           string
		path             string
		wantStatus       int
		wantBodyContains string
	}{
		{
			name:             "serves terminal page",
			path:             "/",
			wantStatus:       http.StatusOK,
			wantBodyContains: "Primary navigation",
		},
		{
			name:             "serves bookmarked conversation",
			path:             "/conversations/bookmarked-id",
			wantStatus:       http.StatusOK,
			wantBodyContains: "Conversations",
		},
		{
			name:             "serves bookmarked Agent",
			path:             "/agents/9e065f6f-3342-4ee3-9443-3c74ec64012d",
			wantStatus:       http.StatusOK,
			wantBodyContains: "New Agent",
		},
		{
			name:             "serves bookmarked Project",
			path:             "/projects/project-id",
			wantStatus:       http.StatusOK,
			wantBodyContains: "Create Project",
		},
		{
			name:             "serves Settings",
			path:             "/settings",
			wantStatus:       http.StatusOK,
			wantBodyContains: "Installation SSH identity",
		},
		{
			name:             "serves terminal assets",
			path:             "/assets/xterm.css",
			wantStatus:       http.StatusOK,
			wantBodyContains: ".xterm",
		},
		{
			name:             "serves app styles",
			path:             "/assets/app.css",
			wantStatus:       http.StatusOK,
			wantBodyContains: ".app-shell",
		},
		{
			name:             "serves app script",
			path:             "/assets/app.js",
			wantStatus:       http.StatusOK,
			wantBodyContains: "terminalSocketURL",
		},
		{
			name:             "serves assets from bookmarked conversation",
			path:             "/conversations/assets/app.js",
			wantStatus:       http.StatusOK,
			wantBodyContains: "terminalSocketURL",
		},
		{
			name:             "serves assets from bookmarked Agent",
			path:             "/agents/assets/app.js",
			wantStatus:       http.StatusOK,
			wantBodyContains: "activeAgentId",
		},
		{
			name:             "serves assets from bookmarked Project",
			path:             "/projects/assets/app.js",
			wantStatus:       http.StatusOK,
			wantBodyContains: "activeProjectId",
		},
		{
			name:             "reports server readiness",
			path:             "/ready",
			wantStatus:       http.StatusOK,
			wantBodyContains: "ready",
		},
		{
			name:       "reports missing page",
			path:       "/missing",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "rejects wrong terminal page method",
			method:     http.MethodPost,
			path:       "/",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "reports missing api endpoint",
			path:       "/api/missing",
			wantStatus: http.StatusNotFound,
		},
		{
			name:             "serves agent api",
			path:             "/api/agents",
			wantStatus:       http.StatusOK,
			wantBodyContains: `"agents"`,
		},
		{
			name:       "rejects wrong conversation tty method",
			method:     http.MethodPost,
			path:       "/api/conversations/not-a-conversation/tty",
			wantStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := startTestServer(t)
			defer shutdownTestServer(t, server)

			client := &http.Client{
				Timeout: 2 * time.Second,
			}
			method := tt.method
			if method == "" {
				method = http.MethodGet
			}

			req, err := http.NewRequestWithContext(
				t.Context(),
				method,
				"http://"+server.Addr+tt.path,
				nil,
			)
			if err != nil {
				t.Fatalf("build %s %s request: %v", method, tt.path, err)
			}

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("%s %s: %v", method, tt.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("%s %s status = %d, want %d", method, tt.path, resp.StatusCode, tt.wantStatus)
			}
			if tt.wantBodyContains == "" {
				return
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read response body: %v", err)
			}
			if !strings.Contains(string(body), tt.wantBodyContains) {
				t.Fatalf("%s %s body = %q, want containing %q", method, tt.path, body, tt.wantBodyContains)
			}
		})
	}
}

func TestStartRejectsInvalidOptions(t *testing.T) {
	tests := []struct {
		name    string
		option  Option
		wantErr string
	}{
		{
			name:    "nil conversation manager",
			option:  WithConversationManager(nil),
			wantErr: "conversation manager must not be nil",
		},
		{
			name:    "nil agent manager",
			option:  WithAgentManager(nil),
			wantErr: "agent manager must not be nil",
		},
		{
			name:    "nil project manager",
			option:  WithProjectManager(nil),
			wantErr: "project manager must not be nil",
		},
		{
			name:    "empty state directory",
			option:  WithStateDir(" \t "),
			wantErr: "state directory is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, err := Start(t.Context(), "127.0.0.1:0", tt.option)
			if err == nil {
				if server != nil {
					shutdownTestServer(t, server)
				}
				t.Fatal("Start() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Start() error = %q, want containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestServerConversationsAPI(t *testing.T) {
	factory := &fakeWrapperFactory{}
	agentManager := newTestAgentManager(t)
	base := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	times := []time.Time{
		base,
		base.Add(time.Minute),
	}
	manager, err := conversations.NewManager(
		t.Context(),
		agentManager,
		conversations.WithStartWrapper(factory.start),
		conversations.WithClock(func() time.Time {
			if len(times) == 0 {
				return base.Add(2 * time.Minute)
			}

			next := times[0]
			times = times[1:]
			return next
		}),
	)
	if err != nil {
		t.Fatalf("conversations.NewManager() error = %v", err)
	}

	server := startTestServer(t, WithAgentManager(agentManager), WithConversationManager(manager))
	defer shutdownTestServer(t, server)

	client := &http.Client{
		Timeout: 2 * time.Second,
	}
	invalidRequests := []struct {
		name string
		body string
	}{
		{name: "malformed json", body: "{"},
		{name: "unknown field", body: `{"unknown":true}`},
		{name: "multiple json values", body: `{"name":"First"} {}`},
	}
	for _, tt := range invalidRequests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := client.Post(
				"http://"+server.Addr+"/api/conversations",
				"application/json",
				strings.NewReader(tt.body),
			)
			if err != nil {
				t.Fatalf("POST invalid conversation: %v", err)
			}
			assertStatus(t, resp, http.StatusBadRequest)
			assertAPIError(t, resp, "invalid conversation request")
		})
	}

	resp, err := client.Get("http://" + server.Addr + "/api/conversations")
	if err != nil {
		t.Fatalf("GET /api/conversations: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	var initial conversationsResponse
	decodeJSON(t, resp, &initial)
	if len(initial.Conversations) != 0 {
		t.Fatalf("initial conversations = %d, want 0", len(initial.Conversations))
	}

	resp, err = client.Post("http://"+server.Addr+"/api/conversations", "application/json", strings.NewReader(`{"name":"   "}`))
	if err != nil {
		t.Fatalf("POST blank conversation: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
	var apiErr errorResponse
	decodeJSON(t, resp, &apiErr)
	if apiErr.Error != "conversation name is required" {
		t.Fatalf("blank conversation error = %q, want conversation name is required", apiErr.Error)
	}

	resp, err = client.Post("http://"+server.Addr+"/api/conversations", "application/json", strings.NewReader(`{"name":"No agent"}`))
	if err != nil {
		t.Fatalf("POST conversation without Agent: %v", err)
	}
	assertStatus(t, resp, http.StatusBadRequest)
	assertAPIError(t, resp, "conversation agent_id is required")

	resp, err = client.Post("http://"+server.Addr+"/api/conversations", "application/json", strings.NewReader(`{"name":"Missing agent","agent_id":"missing"}`))
	if err != nil {
		t.Fatalf("POST conversation with missing Agent: %v", err)
	}
	assertStatus(t, resp, http.StatusBadRequest)
	assertAPIError(t, resp, "conversation agent is invalid")

	first := createConversationViaAPI(t, client, server, "First")
	second := createConversationViaAPI(t, client, server, "Second")
	uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	for _, conversation := range []conversations.Metadata{first, second} {
		if !uuidPattern.MatchString(conversation.ID) {
			t.Fatalf("conversation id %q is not a UUID v4", conversation.ID)
		}
		if conversation.Status != conversations.StatusRunning {
			t.Fatalf("conversation %q status = %q, want %q", conversation.Name, conversation.Status, conversations.StatusRunning)
		}
	}

	resp, err = client.Get("http://" + server.Addr + "/api/conversations")
	if err != nil {
		t.Fatalf("GET /api/conversations after create: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	var listed conversationsResponse
	decodeJSON(t, resp, &listed)
	if len(listed.Conversations) != 2 {
		t.Fatalf("listed conversations = %d, want 2", len(listed.Conversations))
	}
	if listed.Conversations[0].ID != second.ID || listed.Conversations[1].ID != first.ID {
		t.Fatalf("conversation order = [%q, %q], want newest-first [%q, %q]",
			listed.Conversations[0].Name,
			listed.Conversations[1].Name,
			second.Name,
			first.Name,
		)
	}
}

func TestServerDeleteConversationAPI(t *testing.T) {
	factory := &fakeWrapperFactory{}
	manager, agentManager := newTestConversationManager(t, factory.start)
	server := startTestServer(t, WithAgentManager(agentManager), WithConversationManager(manager))
	defer shutdownTestServer(t, server)

	client := &http.Client{
		Timeout: 2 * time.Second,
	}
	conversation := createConversationViaAPI(t, client, server, "delete me")
	if got := factory.wrapperCount(); got != 1 {
		t.Fatalf("started wrappers = %d, want 1", got)
	}

	req, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodDelete,
		"http://"+server.Addr+"/api/conversations/"+conversation.ID,
		nil,
	)
	if err != nil {
		t.Fatalf("new delete request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("DELETE /api/conversations/%s: %v", conversation.ID, err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNoContent)

	select {
	case <-factory.wrapper(0).done:
	case <-time.After(2 * time.Second):
		t.Fatal("deleted conversation wrapper was not shut down")
	}

	resp, err = client.Get("http://" + server.Addr + "/api/conversations")
	if err != nil {
		t.Fatalf("GET /api/conversations after delete: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	var listed conversationsResponse
	decodeJSON(t, resp, &listed)
	if len(listed.Conversations) != 0 {
		t.Fatalf("listed conversations after delete = %d, want 0", len(listed.Conversations))
	}

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("second DELETE /api/conversations/%s: %v", conversation.ID, err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNotFound)
}

func TestServerPersistsConversationMetadata(t *testing.T) {
	stateDir := t.TempDir()
	agentManager, err := agents.NewManager(stateDir)
	if err != nil {
		t.Fatalf("agents.NewManager() error = %v", err)
	}
	factory := &fakeWrapperFactory{}
	manager, err := conversations.NewManager(
		t.Context(),
		agentManager,
		conversations.WithStartWrapper(factory.start),
		conversations.WithStateDir(stateDir),
	)
	if err != nil {
		t.Fatalf("conversations.NewManager() error = %v", err)
	}

	server := startTestServer(t, WithAgentManager(agentManager), WithConversationManager(manager))
	client := &http.Client{
		Timeout: 2 * time.Second,
	}
	conversation := createConversationViaAPI(t, client, server, "persisted")
	defaultAgentID := defaultAgentID(t, agentManager)
	initialStart := factory.startRequest(0)
	if initialStart.SessionID != conversation.ID || initialStart.Harness != harness.TypeClaudeCode {
		t.Fatalf("initial wrapper start = %+v, want id %q", initialStart, conversation.ID)
	}
	wantConfigDir := filepath.Join(stateDir, "agents", defaultAgentID, "config")
	if initialStart.ConfigDir != wantConfigDir {
		t.Fatalf("initial wrapper config dir = %q, want %q", initialStart.ConfigDir, wantConfigDir)
	}
	if conversation.AgentID != defaultAgentID {
		t.Fatalf("created conversation AgentID = %q, want %q", conversation.AgentID, defaultAgentID)
	}
	shutdownTestServer(t, server)

	metadataPath := filepath.Join(stateDir, "conversations", conversation.ID+".json")
	persistedMetadata, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("read persisted conversation metadata: %v", err)
	}
	if bytes.Contains(persistedMetadata, []byte(`"resumable"`)) {
		t.Fatalf("persisted conversation metadata contains obsolete resumable state: %s", persistedMetadata)
	}

	restartedFactory := &fakeWrapperFactory{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/pty" {
				http.NotFound(w, r)
				return
			}

			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close(websocket.StatusNormalClosure, "")
			_, _, _ = conn.Read(r.Context())
		}),
	}
	restartedManager, err := conversations.NewManager(
		t.Context(),
		agentManager,
		conversations.WithStartWrapper(restartedFactory.start),
		conversations.WithStateDir(stateDir),
	)
	if err != nil {
		t.Fatalf("reload conversations.NewManager() error = %v", err)
	}
	restartedServer := startTestServer(t, WithAgentManager(agentManager), WithConversationManager(restartedManager))
	defer shutdownTestServer(t, restartedServer)

	resp, err := client.Get("http://" + restartedServer.Addr + "/api/conversations")
	if err != nil {
		t.Fatalf("GET /api/conversations after restart: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	var listed conversationsResponse
	decodeJSON(t, resp, &listed)
	if len(listed.Conversations) != 1 {
		t.Fatalf("restarted conversations = %d, want 1", len(listed.Conversations))
	}
	if listed.Conversations[0].ID != conversation.ID || listed.Conversations[0].Name != "persisted" {
		t.Fatalf("restarted conversation = %+v, want id %q name persisted", listed.Conversations[0], conversation.ID)
	}
	if listed.Conversations[0].Status != conversations.StatusExited {
		t.Fatalf("restarted conversation status = %q, want %q", listed.Conversations[0].Status, conversations.StatusExited)
	}
	if got := restartedFactory.wrapperCount(); got != 0 {
		t.Fatalf("wrappers started on metadata reload = %d, want 0", got)
	}

	conn, _, err := websocket.Dial(
		t.Context(),
		"ws://"+restartedServer.Addr+"/api/conversations/"+conversation.ID+"/tty",
		nil,
	)
	if err != nil {
		t.Fatalf("dial persisted conversation tty: %v", err)
	}
	if err := conn.Close(websocket.StatusNormalClosure, ""); err != nil {
		t.Fatalf("close persisted conversation tty: %v", err)
	}
	waitForConversationStatus(t, restartedManager, conversation.ID, conversations.StatusRunning)
	if got := restartedFactory.wrapperCount(); got != 1 {
		t.Fatalf("wrappers started after persisted tty connection = %d, want 1", got)
	}
	restoredStart := restartedFactory.startRequest(0)
	if restoredStart.SessionID != conversation.ID || restoredStart.ConfigDir != wantConfigDir {
		t.Fatalf("restored wrapper start = %+v, want id %q", restoredStart, conversation.ID)
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, "http://"+restartedServer.Addr+"/api/conversations/"+conversation.ID, nil)
	if err != nil {
		t.Fatalf("new delete request: %v", err)
	}
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("DELETE persisted conversation: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNoContent)
	if _, err := os.Stat(metadataPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat deleted conversation metadata error = %v, want not exist", err)
	}
}

func TestServerTTYMissingConversation(t *testing.T) {
	factory := &fakeWrapperFactory{}
	manager, agentManager := newTestConversationManager(t, factory.start)

	server := startTestServer(t, WithAgentManager(agentManager), WithConversationManager(manager))
	defer shutdownTestServer(t, server)

	client := &http.Client{
		Timeout: 2 * time.Second,
	}
	resp, err := client.Get("http://" + server.Addr + "/api/conversations/missing/tty")
	if err != nil {
		t.Fatalf("GET missing tty: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNotFound)
}

func TestAppPageUsesProxySafePaths(t *testing.T) {
	page := string(readAsset(t, "assets/index.html"))
	app := string(readAsset(t, "assets/app.js"))
	wants := []string{
		`href="assets/xterm.css"`,
		`href="assets/app.css"`,
		`src="assets/xterm.js"`,
		`src="assets/addon-fit.js"`,
		`src="assets/app.js"`,
	}
	for _, want := range wants {
		if !strings.Contains(page, want) {
			t.Fatalf("terminal page does not contain %q", want)
		}
	}

	if want := `return new URL(path, window.location.origin + basePath)`; !strings.Contains(app, want) {
		t.Fatalf("app script does not contain %q", want)
	}

	for _, bad := range []string{`href="/assets/`, `src="/assets/`, `host + '/pty'`} {
		if strings.Contains(page, bad) || strings.Contains(app, bad) {
			t.Fatalf("terminal page contains proxy-unsafe path %q", bad)
		}
	}

	for _, bad := range []string{`<style>`, `<script>`} {
		if strings.Contains(page, bad) {
			t.Fatalf("terminal page still contains inline asset tag %q", bad)
		}
	}
}

func TestAppScriptUsesConnectionLifecycleStates(t *testing.T) {
	app := string(readAsset(t, "assets/app.js"))
	wants := []string{
		"scheduleReconnect",
		"loadConversations",
		"startConversationPolling",
		"conversationIdFromPath",
		"updateConversationStatuses",
		"setStatus('connected')",
		"setStatus('reconnecting')",
		"setStatus('disconnected')",
		"const reconnectDelays = [1000, 2000, 4000, 8000, 15000]",
		"markConnectionStable",
		"describeSocketClose",
		"pauseAutomaticReconnect",
		"retryConnectionNow",
		"stopAutomaticReconnect",
		"activeSocket.addEventListener('close', (event)",
		"Automatic retries paused after",
		"Cannot reach the Ahh server",
		"connectionBanner.dataset.source === 'data'",
	}
	for _, want := range wants {
		if !strings.Contains(app, want) {
			t.Fatalf("app script does not contain %q", want)
		}
	}
	for _, unwanted := range []string{
		"conversation-exited",
		"conversation.status",
		"const reconnectBaseDelay",
		"const reconnectMaxDelay",
	} {
		if strings.Contains(app, unwanted) {
			t.Fatalf("app script contains backend lifecycle state %q", unwanted)
		}
	}
}

func TestAppScriptResetsTerminalWhenHistoryChangesConversations(t *testing.T) {
	app := string(readAsset(t, "assets/app.js"))
	for _, want := range []string{
		"const conversationChanged = conversationExists && activeConversationId !== route.id;",
		"if (conversationChanged && currentMode === 'conversations') resetTerminalForActiveConversation();",
	} {
		if !strings.Contains(app, want) {
			t.Fatalf("app script does not contain %q", want)
		}
	}
}

func TestAppScriptRestoresPausedReconnectAfterDataRecovery(t *testing.T) {
	app := string(readAsset(t, "assets/app.js"))
	for _, want := range []string{
		"pausedReconnectMessage = message;",
		"const terminalRetryPaused = currentMode === 'conversations' &&",
		"showPausedReconnect();",
	} {
		if !strings.Contains(app, want) {
			t.Fatalf("app script does not contain %q", want)
		}
	}
}

func TestAgentUIUsesIndependentSelectionAndAgentBackedConversationCreation(t *testing.T) {
	page := string(readAsset(t, "assets/index.html"))
	app := string(readAsset(t, "assets/app.js"))
	styles := string(readAsset(t, "assets/app.css"))

	for _, want := range []string{
		`id="conversations-mode-button"`,
		`id="agents-mode-button"`,
		`id="conversation-agent-select"`,
		`id="agent-editor-form"`,
		`id="agent-editor-harness" class="form-control" type="text" readonly`,
		`id="menu-button"`,
		`id="retry-connection-button"`,
		`id="stop-retrying-button"`,
		`id="connection-details-button"`,
		`Start with a name. This Agent will use Claude Code`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("terminal page does not contain %q", want)
		}
	}

	for _, want := range []string{
		"activeConversationId",
		"activeAgentId",
		"'/agents/'",
		"agent_id: agentId",
		"method: 'PATCH'",
		"agentName(conversation.agent_id)",
		"closeSocket();",
		"option.textContent = agent.name",
		"body: JSON.stringify({ name, harness: 'claude-code' })",
	} {
		if !strings.Contains(app, want) {
			t.Fatalf("app script does not contain %q", want)
		}
	}

	for _, want := range []string{".activity-rail", ".editor-region", ".menu-button"} {
		if !strings.Contains(styles, want) {
			t.Fatalf("app styles do not contain %q", want)
		}
	}
	for _, unwanted := range []string{"agent config directory", "agent-id-input", "agent-harness-select"} {
		if strings.Contains(strings.ToLower(page), unwanted) {
			t.Fatalf("Agent editor exposes internal field %q", unwanted)
		}
	}
}

func TestProjectAndSettingsUI(t *testing.T) {
	page := string(readAsset(t, "assets/index.html"))
	app := string(readAsset(t, "assets/app.js"))
	styles := string(readAsset(t, "assets/app.css"))

	for _, want := range []string{
		`id="projects-mode-button"`,
		`class="activity-icon"`,
		`id="settings-mode-button"`,
		`id="project-editor-form"`,
		`placeholder="my-project"`,
		`id="project-source-type"`,
		`<option value="github">github</option>`,
		`This immutable URL-safe name is also the Project ID used in links.`,
		`placeholder="owner/repository"`,
		`id="project-default-branch"`,
		`id="project-refresh-button" class="icon-button compact-icon-button"`,
		`aria-label="Refresh Project"`,
		`id="project-save-message"`,
		`id="project-unavailable-reason"`,
		`id="authentication-mode"`,
		`id="ssh-public-key"`,
		`id="ssh-fingerprint"`,
		`GitHub: add a new SSH key`,
		`Type the current fingerprint to confirm`,
		`GitHub access will fail until the new public key is registered`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("app page does not contain %q", want)
		}
	}
	for _, want := range []string{
		"loadProjects",
		"loadSettings",
		"source: { type: sourceType, repository }",
		"default_branch: { kind, name: nameParts.join(':') }",
		"navigator.clipboard.writeText(sshPublicKey.value)",
		"confirm_fingerprint: confirmFingerprint",
		"regenerateConfirmInput.value !== fingerprint",
		"projectDeleteConfirmInput.value !== project.name",
		"project.unavailable_reason",
		"`${project.source.type} · ${project.source.repository}`",
		"projectRefreshButton.setAttribute('aria-busy', 'true')",
		"deleteButton.className = 'item-delete'",
		"openProjectDeleteDialog(project.id)",
		"'/projects/'",
	} {
		if !strings.Contains(app, want) {
			t.Fatalf("app script does not contain %q", want)
		}
	}
	for _, want := range []string{".activity-rail", ".project-status-pill", ".project-default-branch-field", ".item-delete", ".settings-card"} {
		if !strings.Contains(styles, want) {
			t.Fatalf("app styles do not contain %q", want)
		}
	}
	for _, unwanted := range []string{"Localhost control plane", `class="section-label"`, "Open Conversations"} {
		if strings.Contains(page, unwanted) {
			t.Fatalf("app page contains redundant label %q", unwanted)
		}
	}
}

func TestServerTTYWebSocketProxy(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	input := make(chan string, 1)
	fake := newFakeWrapperServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pty" {
			http.NotFound(w, r)
			return
		}

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		if err := conn.Write(r.Context(), websocket.MessageBinary, []byte("hello\r\n")); err != nil {
			return
		}

		messageType, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		if messageType == websocket.MessageText {
			input <- string(data)
		}
	}))

	manager, agentManager := newTestConversationManager(t, func(context.Context, conversations.WrapperStart) (wrapper.Wrapper, error) {
		return fake, nil
	})
	conversation, err := manager.Create(ctx, "terminal", defaultAgentID(t, agentManager))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	server := startTestServer(t, WithAgentManager(agentManager), WithConversationManager(manager))
	defer shutdownTestServer(t, server)

	conn, _, err := websocket.Dial(ctx, "ws://"+server.Addr+"/api/conversations/"+conversation.ID()+"/tty", nil)
	if err != nil {
		t.Fatalf("dial server websocket: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	messageType, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read server websocket: %v", err)
	}
	if messageType != websocket.MessageBinary {
		t.Fatalf("websocket message type = %v, want %v", messageType, websocket.MessageBinary)
	}
	if string(data) != "hello\r\n" {
		t.Fatalf("websocket data = %q, want %q", data, "hello\r\n")
	}

	const terminalInput = `{"type":"input","data":"pwd\r"}`
	if err := conn.Write(ctx, websocket.MessageText, []byte(terminalInput)); err != nil {
		t.Fatalf("write terminal input: %v", err)
	}
	select {
	case got := <-input:
		if got != terminalInput {
			t.Fatalf("proxied terminal input = %q, want %q", got, terminalInput)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

type fakeWrapperServer struct {
	server    *httptest.Server
	done      chan struct{}
	closeOnce sync.Once
}

type fakeWrapperFactory struct {
	mu       sync.Mutex
	wrappers []*fakeWrapperServer
	starts   []conversations.WrapperStart
	handler  http.Handler
}

func (f *fakeWrapperFactory) start(
	_ context.Context,
	start conversations.WrapperStart,
) (wrapper.Wrapper, error) {
	handler := f.handler
	if handler == nil {
		handler = http.NotFoundHandler()
	}
	fake := newFakeWrapperServer(handler)

	f.mu.Lock()
	defer f.mu.Unlock()

	f.wrappers = append(f.wrappers, fake)
	f.starts = append(f.starts, start)
	return fake, nil
}

func (f *fakeWrapperFactory) wrapperCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.wrappers)
}

func (f *fakeWrapperFactory) wrapper(index int) *fakeWrapperServer {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.wrappers[index]
}

func (f *fakeWrapperFactory) startRequest(index int) conversations.WrapperStart {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.starts[index]
}

func newFakeWrapperServer(handler http.Handler) *fakeWrapperServer {
	return &fakeWrapperServer{
		server: httptest.NewServer(handler),
		done:   make(chan struct{}),
	}
}

func (s *fakeWrapperServer) Address() string {
	return strings.TrimPrefix(s.server.URL, "http://")
}

func (s *fakeWrapperServer) Wait() error {
	<-s.done

	return nil
}

func (s *fakeWrapperServer) Shutdown(context.Context) error {
	s.closeOnce.Do(func() {
		s.server.Close()
		close(s.done)
	})

	return nil
}

func startTestServer(t *testing.T, opts ...Option) *Server {
	t.Helper()

	serverOpts := []Option{WithStateDir(t.TempDir())}
	serverOpts = append(serverOpts, opts...)
	server, err := Start(t.Context(), "127.0.0.1:0", serverOpts...)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	return server
}

func newTestConversationManager(
	t *testing.T,
	startWrapper func(context.Context, conversations.WrapperStart) (wrapper.Wrapper, error),
) (*conversations.Manager, *agents.Manager) {
	t.Helper()

	agentManager := newTestAgentManager(t)
	manager, err := conversations.NewManager(
		t.Context(),
		agentManager,
		conversations.WithStartWrapper(startWrapper),
	)
	if err != nil {
		t.Fatalf("conversations.NewManager() error = %v", err)
	}

	return manager, agentManager
}

func shutdownTestServer(t *testing.T, server *Server) {
	t.Helper()

	if err := server.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if err := server.Wait(); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

func readAsset(t *testing.T, name string) []byte {
	t.Helper()

	data, err := assetsFS.ReadFile(name)
	if err != nil {
		t.Fatalf("read embedded asset %q: %v", name, err)
	}

	return data
}

func createConversationViaAPI(t *testing.T, client *http.Client, server *Server, name string) conversations.Metadata {
	t.Helper()

	requestBody := bytes.NewBufferString(fmt.Sprintf(
		`{"name":%q,"agent_id":%q}`,
		name,
		defaultAgentID(t, server.agents),
	))
	resp, err := client.Post("http://"+server.Addr+"/api/conversations", "application/json", requestBody)
	if err != nil {
		t.Fatalf("POST /api/conversations: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusCreated)

	var conversation conversations.Metadata
	decodeJSON(t, resp, &conversation)
	if conversation.Name != name {
		t.Fatalf("created conversation name = %q, want %q", conversation.Name, name)
	}

	return conversation
}

func newTestAgentManager(t *testing.T) *agents.Manager {
	t.Helper()

	manager, err := agents.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("agents.NewManager() error = %v", err)
	}
	return manager
}

func defaultAgentID(t *testing.T, manager *agents.Manager) string {
	t.Helper()

	listed := manager.List()
	if len(listed) != 1 || listed[0].Name != agents.DefaultAgentName {
		t.Fatalf("Agents = %#v, want one default Agent", listed)
	}
	return listed[0].ID
}

func assertStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()

	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d, body = %q", resp.StatusCode, want, body)
	}
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

func decodeJSON(t *testing.T, resp *http.Response, value any) {
	t.Helper()

	if err := json.NewDecoder(resp.Body).Decode(value); err != nil {
		t.Fatalf("decode JSON response: %v", err)
	}
}

func waitForConversationStatus(t *testing.T, manager *conversations.Manager, id string, want conversations.Status) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, conversation := range manager.List() {
			if conversation.ID == id && conversation.Status == want {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("conversation %q did not reach status %q", id, want)
}
