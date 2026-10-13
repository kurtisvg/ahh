// Package server implements the orchestrator HTTP service.
package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// Server owns the orchestrator HTTP server, wrapper subprocess, and terminal proxies.
// Create one with Start. Wait joins serving and all owned resources.
type Server struct {
	Address string
	Port    int

	http           *http.Server
	wrapper        *wrapperProcess
	cancelRequests context.CancelFunc
	requestsMu     sync.Mutex // Prevents registration after shutdown starts.
	requests       sync.WaitGroup

	done chan struct{}
	err  error // Read after done closes.
}

// Start binds address and port, launches wrap codex in the current directory,
// and serves once the wrapper reports readiness. Port zero selects a free port.
// Cancel ctx or call Shutdown to stop it. Failed startup releases all resources.
func Start(ctx context.Context, address string, port int) (*Server, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate executable: %w", err)
	}
	args := []string{"wrap", "codex", "--address", "127.0.0.1", "--port", "0"}
	return start(ctx, address, port, executable, args...)
}

func start(ctx context.Context, address string, port int, executable string, args ...string) (*Server, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	listenAddress := net.JoinHostPort(address, strconv.Itoa(port))
	var config net.ListenConfig
	listener, err := config.Listen(ctx, "tcp", listenAddress)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	actualAddress, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("parse listener address: %w", err)
	}

	actualPort, err := strconv.Atoi(portText)
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("parse listener port: %w", err)
	}

	child, err := startWrapper(ctx, executable, args...)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}

	requestCtx, cancelRequests := context.WithCancel(ctx)
	protection := http.NewCrossOriginProtection()
	mux := http.NewServeMux()
	mux.Handle("GET /", serveAssets())
	server := &Server{
		Address: actualAddress,
		Port:    actualPort,
		http: &http.Server{
			Handler:           protection.Handler(mux),
			ReadHeaderTimeout: 5 * time.Second,
			BaseContext:       func(net.Listener) context.Context { return requestCtx },
		},
		wrapper:        child,
		cancelRequests: cancelRequests,
		done:           make(chan struct{}),
	}
	mux.HandleFunc("GET /pty", server.proxyPTY(requestCtx, protection))
	go server.serve(ctx, listener)
	return server, nil
}

func (s *Server) serve(ctx context.Context, listener net.Listener) {
	defer close(s.done)
	defer func() {
		s.stopRequests()
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = s.wrapper.Shutdown(shutdown)
		_ = s.wrapper.Wait()
		s.requests.Wait()
	}()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- s.http.Serve(listener)
	}()

	var err error
	select {
	case err = <-serveErr:
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = s.Shutdown(shutdown)
		err = <-serveErr
	}

	// Serve returns ErrServerClosed during normal shutdown; only report unexpected errors.
	if !errors.Is(err, http.ErrServerClosed) {
		s.err = err
	}
}

// Wait joins the HTTP server, wrapper subprocess, and terminal proxy handlers.
// It returns any unexpected HTTP server error.
func (s *Server) Wait() error {
	<-s.done
	return s.err
}

// Shutdown drains requests until ctx expires, then closes remaining connections.
// It is safe to call more than once. Call Wait for wrapper cleanup to finish.
func (s *Server) Shutdown(ctx context.Context) error {
	// HTTP shutdown leaves upgraded connections open; cancel their proxies too.
	s.stopRequests()
	err := s.http.Shutdown(ctx)
	if err != nil {
		_ = s.http.Close()
	}
	return err
}

func (s *Server) stopRequests() {
	s.requestsMu.Lock()
	defer s.requestsMu.Unlock()
	s.cancelRequests()
}
