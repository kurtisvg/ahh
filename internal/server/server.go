package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
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

// Start starts the Ahh HTTP server and the local wrapper it proxies to.
func Start(ctx context.Context, addr string) (*Server, error) {
	return start(ctx, newWrapperSessionManager(), addr)
}

func startWithSessionManager(ctx context.Context, sessions *SessionManager, addr string) (*Server, error) {
	return start(ctx, sessions, addr)
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
	mux.HandleFunc("/api/sessions/", server.serveSessionAPI)

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

func (s *Server) serveSessionAPI(w http.ResponseWriter, r *http.Request) {
	id, action, ok := splitSessionAPIPath(r.URL.Path)
	if !ok || action != "tty" {
		http.NotFound(w, r)
		return
	}

	s.serveTTY(w, r, id)
}

// serveTTY proxies browser websocket traffic to the session wrapper's PTY
// endpoint. The public Ahh API uses tty terminology; the wrapper still owns the
// current PTY transport internally.
func (s *Server) serveTTY(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	sessionWrapper, sessionStatus, ok := s.sessions.Wrapper(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if sessionStatus == SessionStatusExited || sessionWrapper == nil {
		http.Error(w, "session exited", http.StatusGone)
		return
	}

	s.sessions.Touch(id)

	browserConn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	wrapperConn, _, err := websocket.Dial(ctx, "ws://"+sessionWrapper.Address()+"/pty", nil)
	if err != nil {
		_ = browserConn.Close(websocket.StatusTryAgainLater, "wrapper unavailable")
		return
	}

	errCh := make(chan error, 2)
	go func() {
		errCh <- copyWebSocket(ctx, wrapperConn, browserConn)
	}()
	go func() {
		errCh <- copyWebSocket(ctx, browserConn, wrapperConn)
	}()

	err = <-errCh
	cancel()

	closeStatus := websocket.StatusNormalClosure
	reason := ""
	if !normalWebSocketError(err) {
		closeStatus = websocket.StatusInternalError
		reason = "terminal proxy failed"
	}

	_ = browserConn.Close(closeStatus, reason)
	_ = wrapperConn.Close(closeStatus, reason)
	<-errCh
}

func splitSessionAPIPath(path string) (string, string, bool) {
	rest := strings.TrimPrefix(path, "/api/sessions/")
	if rest == path || rest == "" {
		return "", "", false
	}

	id, action, ok := strings.Cut(rest, "/")
	if !ok || id == "" || action == "" || strings.Contains(action, "/") {
		return "", "", false
	}

	return id, action, true
}

func copyWebSocket(ctx context.Context, dst, src *websocket.Conn) error {
	for {
		messageType, data, err := src.Read(ctx)
		if err != nil {
			return err
		}
		if err := dst.Write(ctx, messageType, data); err != nil {
			return err
		}
	}
}

func normalWebSocketError(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, net.ErrClosed) {
		return true
	}

	switch websocket.CloseStatus(err) {
	case websocket.StatusNormalClosure, websocket.StatusGoingAway, websocket.StatusNoStatusRcvd:
		return true
	default:
		return false
	}
}
