package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"time"

	"github.com/kurtisvg/ahh/internal/server/sessions"
)

const (
	shutdownTimeout = 5 * time.Second
)

// Server exposes the Ahh browser surface and proxies live terminal traffic to session wrappers.
type Server struct {
	Addr       string
	httpServer *http.Server
	sessions   *sessions.Manager

	// done is closed after err is set, so Wait can safely read err after
	// receiving from done.
	done chan struct{}
	err  error
}

// Start starts the Ahh HTTP server.
func Start(ctx context.Context, addr string, opts ...Option) (*Server, error) {
	cfg := options{
		stateDir: defaultStateDir(),
	}
	for _, opt := range opts {
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}
	if addr == "" {
		return nil, fmt.Errorf("server listen address is required")
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}
	if cfg.sessions == nil {
		manager, err := sessions.NewManager(ctx, sessions.WithStateDir(cfg.stateDir))
		if err != nil {
			if closeErr := listener.Close(); closeErr != nil {
				return nil, errors.Join(err, fmt.Errorf("close listener: %w", closeErr))
			}
			return nil, err
		}
		cfg.sessions = manager
	}

	server := &Server{
		Addr:     listener.Addr().String(),
		sessions: cfg.sessions,
		done:     make(chan struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", server.serveTerminal)
	mux.HandleFunc("GET /conversations/{id}", server.serveTerminal)
	mux.Handle("/assets/", serveAssets())
	// Relative asset URLs on /conversations/{id} resolve beneath
	// /conversations while preserving any reverse-proxy mount prefix.
	mux.Handle("/conversations/assets/", http.StripPrefix("/conversations", serveAssets()))
	mux.HandleFunc("/ready", server.serveReady)
	mux.Handle("/api/", http.StripPrefix("/api", server.serveAPI()))

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

// serveAPI owns routes under /api after the server strips that prefix.
func (s *Server) serveAPI() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sessions", s.listSessions)
	mux.HandleFunc("POST /sessions", s.createSession)
	mux.HandleFunc("DELETE /sessions/{id}", s.deleteSession)
	mux.HandleFunc("GET /sessions/{id}/tty", s.serveTTY)

	return mux
}

// Wait blocks until the server background loop exits.
func (s *Server) Wait() error {
	<-s.done

	return s.err
}

// Shutdown stops session wrappers and the HTTP server.
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

// serveTerminal serves the embedded browser terminal entrypoint.
func (s *Server) serveTerminal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != "/" && r.Pattern != "GET /conversations/{id}" {
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

// serveReady reports whether the server process is accepting requests.
func (s *Server) serveReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ready\n"))
}
