package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestServerLifecycle(t *testing.T) {
	useTestWrapper(t)

	for _, method := range []string{"cancel", "shutdown"} {
		t.Run(method, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			server, err := startRunning(ctx, "127.0.0.1", 0, os.Args[0], "-test.run=^TestChildWrapper$")
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
			if err != nil || !strings.Contains(string(body), "Ahh terminal") {
				t.Fatalf("response = %q, error = %v", body, err)
			}

			// Leave two attachments open: both can write, and shutdown must close both.
			wsctx, stopSockets := context.WithTimeout(t.Context(), 5*time.Second)
			defer stopSockets()
			var connections []*websocket.Conn
			var harnessPID int
			for i := range 2 {
				conn, _, err := websocket.Dial(wsctx, "ws://"+address+"/pty", nil)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = conn.CloseNow() }()
				connections = append(connections, conn)
				var replay struct {
					Type string `json:"type"`
				}
				if err := wsjson.Read(wsctx, conn, &replay); err != nil || replay.Type != "replay" {
					t.Fatalf("replay message = %+v, error = %v", replay, err)
				}

				input := fmt.Sprintf("proxy-test-%d", i)
				if err := wsjson.Write(wsctx, conn, map[string]string{"type": "input", "data": input + "\n"}); err != nil {
					t.Fatal(err)
				}
				var output strings.Builder
				for !strings.Contains(output.String(), "received: "+input) {
					kind, data, err := conn.Read(wsctx)
					if err != nil || kind != websocket.MessageBinary {
						t.Fatalf("terminal output kind = %v, error = %v", kind, err)
					}
					if len(data) < 8 {
						t.Fatal("terminal output has no offset")
					}
					output.Write(data[8:])
				}

				match := regexp.MustCompile(`pid=(\d+)`).FindStringSubmatch(output.String())
				if len(match) != 2 {
					t.Fatal("harness did not report its PID")
				}
				pid, err := strconv.Atoi(match[1])
				if err != nil {
					t.Fatal(err)
				}
				if i > 0 && pid != harnessPID {
					t.Fatal("attachments connected to different harness processes")
				}
				harnessPID = pid
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
			// Signal 0 checks existence without sending a signal to the process.
			if err := syscall.Kill(harnessPID, 0); !errors.Is(err, syscall.ESRCH) {
				t.Fatalf("harness survived server shutdown: %v", err)
			}
			for _, conn := range connections {
				for {
					if _, _, err := conn.Read(wsctx); err != nil {
						if wsctx.Err() != nil {
							t.Fatal("proxy connection survived shutdown")
						}
						break
					}
				}
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
	server, err := startRunning(ctx, "127.0.0.1", 0, os.Args[0], "-test.run=^TestChildWrapper$")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = server.Wait()
	})

	second, err := startRunning(ctx, server.Address, server.Port, os.Args[0], "-test.run=^TestChildWrapper$")
	if err == nil {
		_ = second.Shutdown(t.Context())
		_ = second.Wait()
		t.Fatal("started on an occupied address")
	}

	cancel()
	if _, err := startRunning(ctx, "127.0.0.1", 0, os.Args[0], "-test.run=^TestChildWrapper$"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled startup = %v", err)
	}
	_ = server.Wait()

	// A failed wrapper handshake must release the already-bound public listener.
	if _, err := startRunning(t.Context(), server.Address, server.Port, "sh", "-c", "echo invalid"); err == nil {
		t.Fatal("started with invalid wrapper startup output")
	}
	address := net.JoinHostPort(server.Address, strconv.Itoa(server.Port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("failed startup left listener open: %v", err)
	}
	_ = listener.Close()
}
