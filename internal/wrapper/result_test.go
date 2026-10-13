package wrapper

import (
	"bytes"
	"context"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestHarnessExitStopsWrapper(t *testing.T) {
	useFakeCodex(t)

	for _, test := range []struct {
		name   string
		input  string
		code   int
		signal string
	}{
		{name: "success", input: "exit\n", code: 0},
		{name: "failure", input: "fail\n", code: 7},
		{name: "signal", code: -1, signal: "terminated"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			server, err := Start(ctx, "127.0.0.1", 0, "codex")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				cancel()
				if err := server.Wait(); err != nil {
					t.Error(err)
				}
			})

			if test.input != "" {
				_, err = io.WriteString(server.harness, test.input)
			} else {
				err = server.harness.Shutdown(ctx)
			}
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-server.harnessDone:
			case <-ctx.Done():
				t.Fatal("exit was not observed")
			}

			if err := server.Wait(); err != nil {
				t.Fatal(err)
			}
			status := server.Result()
			if status.ExitCode == nil || *status.ExitCode != test.code || status.Signal != test.signal {
				t.Fatalf("exit status = %+v", status)
			}
			address := net.JoinHostPort(server.Address, strconv.Itoa(server.Port))
			if conn, err := net.DialTimeout("tcp", address, time.Second); err == nil {
				conn.Close()
				t.Fatal("wrapper listener survived harness exit")
			}
		})
	}
}

func TestExitWithoutKnownCode(t *testing.T) {
	server := &Server{
		done:       make(chan struct{}),
		harnessErr: context.Canceled,
	}
	close(server.done)

	status := server.Result()
	if status.ExitCode != nil || status.Signal != "" {
		t.Fatalf("unknown exit result = %+v", status)
	}
}

func TestHarnessExitBoundsClientDrain(t *testing.T) {
	useFakeCodex(t)
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	server, err := Start(ctx, "127.0.0.1", 0, "codex")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = server.Wait()
	})

	// Leave more replay than the client may receive without acknowledging it.
	_, _ = server.terminal.Write(bytes.Repeat([]byte("x"), 256*1024))
	url := "ws://" + net.JoinHostPort(server.Address, strconv.Itoa(server.Port)) + "/pty"
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.CloseNow() }()
	var replay replayMessage
	if err := wsjson.Read(ctx, conn, &replay); err != nil {
		t.Fatal(err)
	}

	if _, err := io.WriteString(server.harness, "exit\n"); err != nil {
		t.Fatal(err)
	}
	if err := server.Wait(); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != nil {
		t.Fatal("slow client held the wrapper open until cancellation")
	}
}
