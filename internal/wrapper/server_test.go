package wrapper

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestServerLifecycle(t *testing.T) {
	for _, method := range []string{"cancel", "shutdown"} {
		t.Run(method, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			server, err := Start(ctx, "127.0.0.1", 0)
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

			if conn, err := net.DialTimeout("tcp", address, time.Second); err == nil {
				conn.Close()
				t.Fatal("listener remained open")
			}
		})
	}
}

func TestStartFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	server, err := Start(ctx, "127.0.0.1", 0)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = server.Wait()
	})

	if second, err := Start(ctx, server.Address, server.Port); err == nil {
		_ = second.Shutdown(ctx)
		_ = second.Wait()
		t.Fatal("started on an occupied address")
	}

	cancel()
	if _, err := Start(ctx, "127.0.0.1", 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled startup = %v", err)
	}
}
