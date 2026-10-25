package server

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/kurtisvg/ahh/internal/wrapper"
)

func useTestWrapper(t *testing.T) {
	t.Helper()

	data, err := os.ReadFile("../harness/pty/testdata/fake-harness")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "codex"), data, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AHH_TEST_WRAPPER", "1")
}

func TestChildWrapper(t *testing.T) {
	if os.Getenv("AHH_TEST_WRAPPER") != "1" {
		return
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	server, err := wrapper.Start(ctx, "127.0.0.1", 0, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(struct {
		Address string `json:"address"`
		Port    int    `json:"port"`
	}{server.Address, server.Port}); err != nil {
		t.Error(err)
		cancel()
	}
	if err := server.Wait(); err != nil {
		t.Error(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(struct {
		Type   string             `json:"type"`
		Result wrapper.ExitResult `json:"result"`
	}{Type: "exit", Result: server.Result()}); err != nil {
		t.Error(err)
	}
}

func TestWrapperSubprocessAndShutdown(t *testing.T) {
	useTestWrapper(t)

	p, err := startWrapper(t.Context(), "", os.Args[0], "-test.run=^TestChildWrapper$")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = p.Shutdown(context.Background())
		_ = p.Wait()
	})
	if p.command.Process.Pid == os.Getpid() {
		t.Fatal("wrapper is not a subprocess")
	}

	for range 2 {
		if err := p.Shutdown(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	_ = p.Wait()
	if p.command.ProcessState == nil || !p.command.ProcessState.Exited() {
		t.Fatal("wrapper was not reaped")
	}
	if conn, err := net.DialTimeout("tcp", p.url.Host, time.Second); err == nil {
		conn.Close()
		t.Fatal("wrapper listener survived shutdown")
	}
}

func TestWrapperStartupFailure(t *testing.T) {
	for _, script := range []string{
		"exit 1",
		"echo invalid; exec sleep 30",
		`echo '{"address":"0.0.0.0","port":8080}'; exec sleep 30`,
		`echo '{"address":"127.0.0.1","port":0}'; exec sleep 30`,
		"exec sleep 30",
	} {
		t.Run(script, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()

			if _, err := startWrapper(ctx, "", "sh", "-c", script); err == nil {
				t.Fatal("expected startup failure")
			}
		})
	}
}
