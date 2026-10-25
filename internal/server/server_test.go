package server

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestServerLifecycle(t *testing.T) {
	useTestWrapper(t)

	for _, method := range []string{"cancel", "shutdown"} {
		t.Run(method, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			server, err := start(ctx, "127.0.0.1", 0, os.Args[0], "-test.run=^TestChildWrapper$")
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
				t.Fatal("server did not stop")
			}

			if server.wrapper.command.ProcessState == nil {
				t.Fatal("Wait returned before the wrapper was reaped")
			}
			if conn, err := net.DialTimeout("tcp", address, time.Second); err == nil {
				conn.Close()
				t.Fatal("listener remained open")
			}
		})
	}
}

func TestStartFailure(t *testing.T) {
	useTestWrapper(t)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	server, err := start(ctx, "127.0.0.1", 0, os.Args[0], "-test.run=^TestChildWrapper$")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = server.Wait()
	})

	second, err := start(ctx, server.Address, server.Port, os.Args[0], "-test.run=^TestChildWrapper$")
	if err == nil {
		_ = second.Shutdown(t.Context())
		_ = second.Wait()
		t.Fatal("started on an occupied address")
	}

	cancel()
	if _, err := start(ctx, "127.0.0.1", 0, os.Args[0], "-test.run=^TestChildWrapper$"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled startup = %v", err)
	}
	_ = server.Wait()

	// A failed wrapper handshake must release the already-bound public listener.
	if _, err := start(t.Context(), server.Address, server.Port, "sh", "-c", "echo invalid"); err == nil {
		t.Fatal("started with invalid wrapper startup output")
	}
	address := net.JoinHostPort(server.Address, strconv.Itoa(server.Port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("failed startup left listener open: %v", err)
	}
	_ = listener.Close()
}
