// Package wrapper serves HTTP and owns the lifetime of a running harness.
//
// This layer owns the single output reader and input coordinator, along with
// browser connections, output history, and broadcast queues. It serializes input
// writes and resize commands, drains final output, and releases the harness terminal.
// Every attachment may send input; the last resize command sets the shared size.
package wrapper

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/kurtisvg/ahh/internal/harness"
)

// Server owns the wrapper HTTP server and a running harness. Create one with Start.
// It also owns the harness output reader. Shutdown initiates cleanup; Wait joins
// serving, harness cleanup, and output copying. Harness exit stops the wrapper.
type Server struct {
	Address string
	Port    int

	http          *http.Server
	harness       harness.Harness
	terminal      *terminal
	cancelSockets context.CancelFunc
	harnessDone   chan struct{}
	harnessErr    error // Read after harnessDone closes.

	inputMu   sync.Mutex // Orders input and resize commands from all connections.
	socketsMu sync.Mutex // Prevents new handlers from starting during shutdown.
	sockets   sync.WaitGroup
	draining  bool // Protected by socketsMu; rejects new attachments during final drain.

	done chan struct{}
	err  error // Read after done closes.
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

	socketCtx, cancelSockets := context.WithCancel(ctx)
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
		harness:       h,
		terminal:      newTerminal(),
		cancelSockets: cancelSockets,
		harnessDone:   make(chan struct{}),
		done:          make(chan struct{}),
	}
	mux.HandleFunc("GET /pty", server.servePTY(socketCtx))
	go server.serve(ctx, listener)
	return server, nil
}

func (s *Server) serve(ctx context.Context, listener net.Listener) {
	defer close(s.done)

	outputDone := make(chan error, 1)
	go func() {
		err := copyHarnessOutput(ctx, s.harness, s.terminal)
		// Close queues after the last output has been saved.
		s.terminal.finish()
		// Publish confirmed exit only after the final output has been drained.
		s.harnessErr = s.harness.Wait()
		close(s.harnessDone)
		outputDone <- err
	}()

	// Finish all cleanup before Wait returns.
	defer func() {
		s.stopSockets()
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = s.harness.Shutdown(shutdown)
		_ = s.harness.Wait()
		s.err = errors.Join(s.err, <-outputDone)
		s.sockets.Wait()
	}()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- s.http.Serve(listener)
	}()

	var err error
	select {
	case err = <-serveErr:
	case <-s.harnessDone:
		// Stop accepting attachments, then give existing clients time to drain.
		s.socketsMu.Lock()
		s.draining = true
		s.socketsMu.Unlock()

		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := s.http.Shutdown(shutdown); err != nil {
			_ = s.http.Close()
		}
		drained := make(chan struct{})
		go func() {
			s.sockets.Wait()
			close(drained)
		}()
		select {
		case <-drained:
		case <-shutdown.Done():
			s.stopSockets()
			<-drained
		case <-ctx.Done():
			s.stopSockets()
			<-drained
		}
		err = <-serveErr
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

// Wait blocks until the server, harness, output reader, and WebSocket handlers stop.
// It returns any HTTP server or output-copy error.
func (s *Server) Wait() error {
	<-s.done
	return s.err
}

// Shutdown drains requests until ctx expires, then closes remaining connections.
// It is safe to call more than once. Call Wait for cleanup to finish.
func (s *Server) Shutdown(ctx context.Context) error {
	// HTTP shutdown leaves WebSockets open, so cancel their handlers explicitly.
	s.stopSockets()
	err := s.http.Shutdown(ctx)
	if err != nil {
		_ = s.http.Close()
	}
	return err
}

func (s *Server) stopSockets() {
	// Hold the lock so no handler can register after cancellation.
	s.socketsMu.Lock()
	s.cancelSockets()
	s.socketsMu.Unlock()
}
