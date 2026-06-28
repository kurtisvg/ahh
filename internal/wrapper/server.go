package wrapper

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/kurtisvg/ahh/internal/harness"
)

const shutdownTimeout = 5 * time.Second

// Server exposes the wrapper API for a running harness.
type Server struct {
	addr       string
	httpServer *http.Server
	terminal   *terminalSession
	done       chan struct{}
	runErr     error
}

// Start starts the wrapper HTTP server.
func Start(h harness.Harness, addr string) (*Server, error) {
	if addr == "" {
		return nil, fmt.Errorf("wrapper address is required")
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}

	server := &Server{
		addr:     listener.Addr().String(),
		terminal: newTerminalSession(h),
		done:     make(chan struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ready", serveReady(h))
	mux.Handle("/pty", servePTY(server.terminal))

	server.httpServer = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		defer close(server.done)

		err := server.httpServer.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			server.runErr = err
		}
	}()

	return server, nil
}

func (s *Server) URL() string {
	return "http://" + s.addr
}

func (s *Server) Wait() error {
	<-s.done

	return s.runErr
}

func (s *Server) Close(ctx context.Context) error {
	s.terminal.Close()

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("shutdown wrapper server: %w", err)
	}

	return nil
}

// serveReady reports whether this wrapper still has a running harness process.
// It is a lifecycle check for the wrapper transport, not a deeper terminal
// responsiveness check.
func serveReady(h harness.Harness) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if h.Done() {
			http.Error(w, "harness stopped", http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ready\n"))
	}
}
