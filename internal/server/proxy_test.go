package server

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestShutdownWithStalledTerminalClient(t *testing.T) {
	useTestWrapper(t)

	for _, method := range []string{"cancel", "shutdown"} {
		t.Run(method, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			server, err := startRunning(t,
				ctx,
				"127.0.0.1",
				0,
				os.Args[0],
				"-test.run=^TestChildWrapper$",
			)
			if err != nil {
				t.Fatal(err)
			}

			// An unbuffered connection makes output block as soon as the client stops reading.
			frontend, client := net.Pipe()
			writes := make(chan struct{}, 2)
			writer := &hijackWriter{
				ResponseRecorder: httptest.NewRecorder(),
				conn: &observedConn{
					Conn:   frontend,
					writes: writes,
				},
			}
			handlerDone := make(chan struct{})
			t.Cleanup(func() {
				cancel()
				_ = client.Close()
				_ = frontend.Close()
				_ = server.Wait()
				<-handlerDone
			})

			address := net.JoinHostPort(server.Address, strconv.Itoa(server.Port))
			request := httptest.NewRequestWithContext(
				server.http.BaseContext(nil),
				http.MethodGet,
				"http://"+address+"/pty",
				nil,
			)
			request.Header.Set("Connection", "Upgrade")
			request.Header.Set("Upgrade", "websocket")
			request.Header.Set("Sec-WebSocket-Version", "13")
			request.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

			go func() {
				defer close(handlerDone)
				server.http.Handler.ServeHTTP(writer, request)
			}()

			if err := client.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
				t.Fatal(err)
			}
			response, err := http.ReadResponse(bufio.NewReader(client), request)
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
			if response.StatusCode != http.StatusSwitchingProtocols {
				t.Fatalf("upgrade status = %d", response.StatusCode)
			}

			<-writes // The upgrade response was the first write.
			select {
			case <-writes: // Terminal output is now blocked on the non-reading client.
			case <-time.After(5 * time.Second):
				t.Fatal("proxy did not start writing terminal output")
			}

			if method == "cancel" {
				cancel()
			} else {
				shutdown, stop := context.WithTimeout(t.Context(), time.Second)
				defer stop()
				if err := server.Shutdown(shutdown); err != nil {
					t.Fatal(err)
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
				t.Fatal("stalled terminal client prevented shutdown")
			}
		})
	}
}

// hijackWriter gives the real proxy handler an unbuffered frontend connection.
type hijackWriter struct {
	*httptest.ResponseRecorder
	conn net.Conn
}

func (w *hijackWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.conn, bufio.NewReadWriter(bufio.NewReader(w.conn), bufio.NewWriter(w.conn)), nil
}

// observedConn reports when the proxy attempts a write, without consuming output.
type observedConn struct {
	net.Conn
	writes chan struct{}
}

func (c *observedConn) Write(data []byte) (int, error) {
	select {
	case c.writes <- struct{}{}:
	default:
	}
	return c.Conn.Write(data)
}
