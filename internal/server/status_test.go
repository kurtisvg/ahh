package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWrapperExitReport(t *testing.T) {
	for _, test := range []struct {
		name   string
		report string
		state  string
		code   int
		signal string
	}{
		{name: "success", report: `{"type":"exit","result":{"exit_code":0}}`, state: "exited"},
		{name: "failure", report: `{"type":"exit","result":{"exit_code":7}}`, state: "exited", code: 7},
		{name: "signal", report: `{"type":"exit","result":{"exit_code":-1,"signal":"terminated"}}`, state: "exited", code: -1, signal: "terminated"},
		{name: "missing", state: "failed"},
		{name: "malformed", report: `{"type":"exit","result":null}`, state: "failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()

			// Both records can arrive in one read, before startup returns.
			script := `printf '%s\n' '{"address":"127.0.0.1","port":12345}' '` + test.report + `'`
			child, err := startWrapper(ctx, "", "sh", "-c", script)
			if err != nil {
				t.Fatal(err)
			}
			err = child.Wait()
			if (err != nil) != (test.state == "failed") {
				t.Fatalf("wrapper result error = %v", err)
			}

			manager := newExecutionManager(func(context.Context, string) (*wrapperProcess, error) {
				return child, nil
			})
			if err := manager.startAndWait(ctx, "", ""); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = manager.Close(t.Context()) })
			server := &Server{executions: manager}
			response := httptest.NewRecorder()
			server.serveStatus(response, httptest.NewRequest(http.MethodGet, "/status", nil))
			var status processStatus
			if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
				t.Fatal(err)
			}
			if status.State != test.state || status.Signal != test.signal {
				t.Fatalf("status = %+v", status)
			}
			if test.state == "exited" {
				if status.ExitCode == nil || *status.ExitCode != test.code {
					t.Fatalf("exit code = %v", status.ExitCode)
				}
			} else if status.ExitCode != nil {
				t.Fatal("invented a harness exit code for a missing report")
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("status response can be cached")
			}
		})
	}
}
