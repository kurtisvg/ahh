package wrapper

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestServerLifecycle(t *testing.T) {
	useFakeCodex(t)

	for _, method := range []string{"cancel", "shutdown"} {
		t.Run(method, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			server, err := Start(
				ctx,
				"127.0.0.1",
				0,
				"codex",
			)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				cancel()
				_ = server.Wait()
			})

			address := net.JoinHostPort(server.Address, strconv.Itoa(server.Port))
			client := &http.Client{Timeout: time.Second}
			resp, err := client.Get("http://" + address)
			if err != nil {
				t.Fatal(err)
			}

			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || string(body) != "hello world\n" {
				t.Fatalf("response = %q, error = %v", body, err)
			}

			if method == "cancel" {
				cancel()
			} else {
				shutdown, stop := context.WithTimeout(t.Context(), time.Second)
				defer stop()
				for range 2 {
					if err := server.Shutdown(shutdown); err != nil {
						t.Fatal(err)
					}
				}
			}

			done := make(chan error, 1)
			go func() { done <- server.Wait() }()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("Wait did not return after shutdown")
			}

			// Server completion includes harness cleanup, even without canceling ctx.
			harnessDone := make(chan struct{})
			go func() {
				_ = server.harness.Wait()
				close(harnessDone)
			}()
			select {
			case <-harnessDone:
			case <-time.After(time.Second):
				t.Fatal("Wait returned before harness cleanup")
			}

			if conn, err := net.DialTimeout("tcp", address, time.Second); err == nil {
				conn.Close()
				t.Fatal("listener remained open")
			}
		})
	}
}

func TestStartFailure(t *testing.T) {
	useFakeCodex(t)

	ctx, cancel := context.WithCancel(t.Context())
	server, err := Start(
		ctx,
		"127.0.0.1",
		0,
		"codex",
	)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = server.Wait()
	})

	second, err := Start(
		ctx,
		server.Address,
		server.Port,
		"codex",
	)
	if err == nil {
		_ = second.Shutdown(ctx)
		_ = second.Wait()
		t.Fatal("started on an occupied address")
	}

	cancel()
	_, err = Start(
		ctx,
		"127.0.0.1",
		0,
		"codex",
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled startup = %v", err)
	}
}

func TestLaunchFailureClosesListener(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	// Reserve an address so failed startup can be checked for listener leaks.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}

	server, err := Start(
		t.Context(),
		host,
		port,
		"codex",
	)
	if err == nil {
		_ = server.Shutdown(t.Context())
		_ = server.Wait()
		t.Fatal("started without a Codex executable")
	}
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("expected missing executable error, got %v", err)
	}

	listener, err = net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("listener was not released: %v", err)
	}
	_ = listener.Close()
}

func useFakeCodex(t *testing.T) {
	t.Helper()

	script, err := os.ReadFile("../harness/process/testdata/fake-harness")
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "codex"), script, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
}
