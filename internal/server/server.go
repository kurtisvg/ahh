package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/kurtisvg/ahh/internal/harness"
	"github.com/kurtisvg/ahh/internal/wrapper"
)

const (
	defaultWrapperAddr = "127.0.0.1:0"
	shutdownTimeout    = 5 * time.Second
)

// Server exposes the Ahh browser surface and proxies live terminal traffic to a wrapper.
type Server struct {
	listenAddr    string
	wrapperServer string
	httpServer    *http.Server
	wrapper       *wrapper.Server
	harness       harness.Harness
	client        *http.Client
	httpErr       chan error
	done          chan struct{}
	runErr        error
}

// Start starts the Ahh HTTP server and the local wrapper it proxies to.
func Start(ctx context.Context, listenAddr string) (*Server, error) {
	h, err := harness.Start(ctx)
	if err != nil {
		return nil, fmt.Errorf("start claude-code harness: %w", err)
	}

	wrapperServer, err := wrapper.Start(h, defaultWrapperAddr)
	if err != nil {
		h.Close()
		return nil, fmt.Errorf("start wrapper server: %w", err)
	}

	server, err := start(ctx, wrapperServer.URL(), listenAddr, h, wrapperServer)
	if err != nil {
		_ = wrapperServer.Close(context.Background())
		return nil, err
	}

	return server, nil
}

func startWithWrapperServer(ctx context.Context, wrapperServer string, listenAddr string) (*Server, error) {
	return start(ctx, wrapperServer, listenAddr, nil, nil)
}

func start(
	ctx context.Context,
	wrapperServer string,
	listenAddr string,
	h harness.Harness,
	wrapperServerProcess *wrapper.Server,
) (*Server, error) {
	if listenAddr == "" {
		return nil, fmt.Errorf("server listen address is required")
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", listenAddr, err)
	}

	server := &Server{
		listenAddr:    listener.Addr().String(),
		wrapperServer: strings.TrimRight(wrapperServer, "/"),
		wrapper:       wrapperServerProcess,
		harness:       h,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		httpErr: make(chan error, 1),
		done:    make(chan struct{}),
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
		err := server.httpServer.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			server.httpErr <- err
			return
		}
		server.httpErr <- nil
	}()

	go server.wait(ctx)

	return server, nil
}

func (s *Server) URL() string {
	return "http://" + s.listenAddr
}

func (s *Server) Wait() error {
	<-s.done

	return s.runErr
}

func (s *Server) Close(ctx context.Context) error {
	if s.wrapper != nil {
		if err := s.wrapper.Close(ctx); err != nil {
			return fmt.Errorf("close wrapper server: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("shutdown server: %w", err)
	}

	return nil
}

func (s *Server) wait(ctx context.Context) {
	defer close(s.done)

	harnessErr := s.waitForHarness(ctx)
	wrapperErr := s.waitForWrapper()

	select {
	case err := <-s.httpErr:
		if err != nil {
			s.runErr = fmt.Errorf("serve http: %w", err)
		}
	case err := <-harnessErr:
		if err != nil {
			s.runErr = fmt.Errorf("run claude-code harness: %w", err)
		}
	case err := <-wrapperErr:
		if err != nil {
			s.runErr = fmt.Errorf("serve wrapper: %w", err)
		}
	case <-ctx.Done():
		s.runErr = ctx.Err()
	}
}

func (s *Server) waitForHarness(ctx context.Context) <-chan error {
	ch := make(chan error, 1)
	if s.harness == nil {
		return ch
	}

	go func() {
		ch <- s.harness.Wait(ctx)
	}()

	return ch
}

func (s *Server) waitForWrapper() <-chan error {
	ch := make(chan error, 1)
	if s.wrapper == nil {
		return ch
	}

	go func() {
		ch <- s.wrapper.Wait()
	}()

	return ch
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

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, s.wrapperServerHTTPURL("/ready"), nil)
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

	wrapperConn, _, err := websocket.Dial(ctx, s.wrapperServerWebSocketURL("/pty"), nil)
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

func (s *Server) wrapperServerHTTPURL(path string) string {
	return s.wrapperServer + path
}

func (s *Server) wrapperServerWebSocketURL(path string) string {
	switch {
	case strings.HasPrefix(s.wrapperServer, "http://"):
		return "ws://" + strings.TrimPrefix(s.wrapperServer, "http://") + path
	case strings.HasPrefix(s.wrapperServer, "https://"):
		return "wss://" + strings.TrimPrefix(s.wrapperServer, "https://") + path
	default:
		return s.wrapperServer + path
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
