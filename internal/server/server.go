// Package server implements the orchestrator HTTP service.
package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"
)

// Server owns a running orchestrator HTTP server. Create one with Start.
type Server struct {
	Address string
	Port    int

	http *http.Server
	done chan struct{}
	err  error // Read after done closes.
}

// Start binds address and port, then starts serving. Port zero selects a free port.
// Cancel ctx or call Shutdown to stop it.
func Start(ctx context.Context, address string, port int) (*Server, error) {
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

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello world\n"))
	})
	server := &Server{
		Address: actualAddress,
		Port:    actualPort,
		http: &http.Server{
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
		done: make(chan struct{}),
	}
	go server.serve(ctx, listener)
	return server, nil
}

func (s *Server) serve(ctx context.Context, listener net.Listener) {
	defer close(s.done)

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

// Wait blocks until serving ends and returns any unexpected server error.
func (s *Server) Wait() error {
	<-s.done
	return s.err
}

// Shutdown drains requests until ctx expires, then closes remaining connections.
// It is safe to call more than once. Call Wait for serving to finish.
func (s *Server) Shutdown(ctx context.Context) error {
	err := s.http.Shutdown(ctx)
	if err != nil {
		_ = s.http.Close()
	}
	return err
}
