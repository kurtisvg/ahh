// Package wrapper serves HTTP and owns the lifetime of a running harness.
//
// This layer owns the single output reader and input coordinator, along with
// browser connections, output history, and broadcast queues. It serializes input
// writes and resize commands, drains final output, and releases the harness terminal.
package wrapper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/kurtisvg/ahh/internal/harness"
)

// Server owns the wrapper HTTP server and a running harness. Create one with Start.
// It also owns the harness output reader. Shutdown initiates cleanup; Wait joins
// serving, harness cleanup, and output copying. Harness exit alone does not stop HTTP.
type Server struct {
	Address string
	Port    int

	http    *http.Server
	harness harness.Harness
	done    chan struct{}
	err     error // Published by closing done.
}

// Start binds address and port, then starts and owns the named harness.
// Port zero selects a free port. Cancel ctx or call Shutdown to stop serving.
func Start(ctx context.Context, address string, port int, name string) (*Server, error) {
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

	h, err := harness.Start(ctx, name)
	if err != nil {
		_ = listener.Close()
		return nil, err
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
		harness: h,
		done:    make(chan struct{}),
	}
	go server.serve(ctx, listener)
	return server, nil
}

func (s *Server) serve(ctx context.Context, listener net.Listener) {
	defer close(s.done)

	outputDone := make(chan error, 1)
	go func() { outputDone <- copyHarnessOutput(ctx, s.harness, io.Discard) }()

	// Finish harness cleanup before releasing callers of Wait.
	defer func() {
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = s.harness.Shutdown(shutdown)
		_ = s.harness.Wait()
		s.err = errors.Join(s.err, <-outputDone)
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

// Wait joins serving, process cleanup, and output copying, returning server or copy errors.
func (s *Server) Wait() error {
	<-s.done
	return s.err
}

// Shutdown drains requests until ctx expires, then closes remaining connections.
// It is safe to call more than once. Call Wait to join serving and process cleanup.
func (s *Server) Shutdown(ctx context.Context) error {
	err := s.http.Shutdown(ctx)
	if err != nil {
		_ = s.http.Close()
	}
	return err
}
