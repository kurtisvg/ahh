package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"time"

	"github.com/kurtisvg/ahh/internal/wrapper"
)

const (
	defaultWrapperAddr    = "127.0.0.1:0"
	defaultWrapperHarness = wrapper.ClaudeCodeHarness
	shutdownTimeout       = 5 * time.Second
)

// Server exposes the Ahh browser surface and proxies live terminal traffic to a wrapper.
type Server struct {
	Addr       string
	httpServer *http.Server
	sessions   *SessionManager

	// done is closed after err is set, so Wait can safely read err after
	// receiving from done.
	done chan struct{}
	err  error
}

// Start starts the Ahh HTTP server.
func Start(ctx context.Context, addr string, opts ...Option) (*Server, error) {
	cfg := options{
		sessions: newWrapperSessionManager(),
	}
	for _, opt := range opts {
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}

	return start(ctx, cfg.sessions, addr)
}

func start(ctx context.Context, sessions *SessionManager, addr string) (*Server, error) {
	if addr == "" {
		return nil, fmt.Errorf("server listen address is required")
	}
	if sessions == nil {
		return nil, fmt.Errorf("session manager is required")
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}

	server := &Server{
		Addr:     listener.Addr().String(),
		sessions: sessions,
		done:     make(chan struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", server.serveTerminal)
	mux.Handle("/assets/", serveAssets())
	mux.HandleFunc("/ready", server.serveReady)
	mux.Handle("/api/", http.StripPrefix("/api", server.apiHandler()))

	server.httpServer = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		defer close(server.done)

		serveErr := make(chan error, 1)
		go func() {
			err := server.httpServer.Serve(listener)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				serveErr <- err
				return
			}
			serveErr <- nil
		}()

		select {
		case err := <-serveErr:
			if err != nil {
				server.err = fmt.Errorf("serve http: %w", err)
			}
		case <-ctx.Done():
			server.err = ctx.Err()
		}
	}()

	return server, nil
}

func (s *Server) Wait() error {
	<-s.done

	return s.err
}

// Shutdown stops the wrapper and HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()

	if err := s.sessions.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown sessions: %w", err)
	}

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("shutdown server: %w", err)
	}

	return nil
}

func (s *Server) serveTerminal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	page, err := fs.ReadFile(assetsFS, "assets/index.html")
	if err != nil {
		http.Error(w, "terminal page unavailable", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(page)
}

func (s *Server) serveReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ready\n"))
}
