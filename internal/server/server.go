package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
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
	wrapper    wrapper.Wrapper
	client     *http.Client

	// done is closed after err is set, so Wait can safely read err after
	// receiving from done.
	done chan struct{}
	err  error
}

// Start starts the Ahh HTTP server and the local wrapper it proxies to.
func Start(ctx context.Context, addr string) (*Server, error) {
	w, err := wrapper.Start(ctx, defaultWrapperHarness, defaultWrapperAddr)
	if err != nil {
		return nil, fmt.Errorf("start wrapper server: %w", err)
	}

	server, err := start(ctx, w, addr)
	if err != nil {
		_ = w.Shutdown(context.Background())
		return nil, err
	}

	return server, nil
}

func startWithWrapper(ctx context.Context, w wrapper.Wrapper, addr string) (*Server, error) {
	return start(ctx, w, addr)
}

func start(ctx context.Context, w wrapper.Wrapper, addr string) (*Server, error) {
	if addr == "" {
		return nil, fmt.Errorf("server listen address is required")
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}

	server := &Server{
		Addr:    listener.Addr().String(),
		wrapper: w,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		done: make(chan struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", server.serveTerminal)
	mux.Handle("/assets/", serveAssets())
	mux.HandleFunc("/ready", server.serveReady)
	mux.HandleFunc("/pty", server.servePTY)

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

		wrapperErr := make(chan error, 1)
		go func() {
			wrapperErr <- server.wrapper.Wait()
		}()

		select {
		case err := <-serveErr:
			if err != nil {
				server.err = fmt.Errorf("serve http: %w", err)
			}
		case err := <-wrapperErr:
			if err != nil {
				server.err = fmt.Errorf("serve wrapper: %w", err)
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

	if err := s.wrapper.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown wrapper server: %w", err)
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

// serveReady proxies wrapper readiness so browser clients only talk to the Ahh
// server. It reports readiness for the prototype wrapper transport, not a
// Conversation resource.
func (s *Server) serveReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "http://"+s.wrapper.Address()+"/ready", nil)
	if err != nil {
		http.Error(w, "wrapper readiness request failed", http.StatusInternalServerError)
		return
	}

	resp, err := s.client.Do(req)
	if err != nil {
		http.Error(w, "wrapper unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// servePTY proxies browser websocket traffic to the wrapper PTY endpoint. This
// keeps the browser on the Ahh server boundary instead of exposing the wrapper
// address directly.
func (s *Server) servePTY(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	browserConn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	wrapperConn, _, err := websocket.Dial(ctx, "ws://"+s.wrapper.Address()+"/pty", nil)
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

	status := websocket.StatusNormalClosure
	reason := ""
	if !normalWebSocketError(err) {
		status = websocket.StatusInternalError
		reason = "terminal proxy failed"
	}

	_ = browserConn.Close(status, reason)
	_ = wrapperConn.Close(status, reason)
	<-errCh
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
