package wrapper

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/kurtisvg/ahh/internal/harness"
)

const shutdownTimeout = 5 * time.Second

// Wrapper manages a running harness wrapper.
type Wrapper interface {
	Address() string
	Wait() error
	Shutdown(context.Context) error
}

type options struct {
	resume bool
}

// Option configures a wrapper and its harness process.
type Option func(*options)

// WithResume configures the harness to resume the requested session.
func WithResume() Option {
	return func(opts *options) {
		opts.resume = true
	}
}

// Server exposes the wrapper API for a running harness.
type Server struct {
	Addr       string
	httpServer *http.Server
	harness    harness.Harness
	terminal   *terminalSession

	// done is closed after err is set, so Wait can safely read err after
	// receiving from done.
	done chan struct{}
	err  error
}

var _ Wrapper = (*Server)(nil)

// Start starts the wrapper HTTP server for the requested harness.
func Start(
	ctx context.Context,
	harnessType harness.Type,
	addr string,
	sessionID string,
	opts ...Option,
) (*Server, error) {
	parsedID, err := uuid.Parse(sessionID)
	if err != nil || parsedID == uuid.Nil {
		return nil, fmt.Errorf("invalid session id %q", sessionID)
	}
	sessionID = parsedID.String()

	cfg := options{}
	for _, opt := range opts {
		opt(&cfg)
	}

	h, err := startHarness(ctx, harnessType, sessionID, cfg.resume)
	if err != nil {
		return nil, err
	}

	server, err := start(ctx, h, addr)
	if err != nil {
		h.Close()
		return nil, err
	}

	return server, nil
}

func startHarness(
	ctx context.Context,
	harnessType harness.Type,
	sessionID string,
	resume bool,
) (harness.Harness, error) {
	switch harnessType {
	case harness.ClaudeCode:
		var opts []harness.Option
		if resume {
			opts = append(opts, harness.WithResume())
		}
		h, err := harness.Start(ctx, sessionID, opts...)
		if err != nil {
			return nil, fmt.Errorf("start %s harness: %w", harnessType, err)
		}

		return h, nil
	default:
		return nil, fmt.Errorf("unsupported harness %q", harnessType)
	}
}

func start(ctx context.Context, h harness.Harness, addr string) (*Server, error) {
	if addr == "" {
		return nil, fmt.Errorf("wrapper address is required")
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}

	server := &Server{
		Addr:     listener.Addr().String(),
		harness:  h,
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

		serveErr := make(chan error, 1)
		go func() {
			err := server.httpServer.Serve(listener)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				serveErr <- err
				return
			}
			serveErr <- nil
		}()

		harnessErr := make(chan error, 1)
		go func() {
			harnessErr <- server.harness.Wait(ctx)
		}()

		select {
		case err := <-serveErr:
			if err != nil {
				server.err = fmt.Errorf("serve wrapper http: %w", err)
			}
		case err := <-harnessErr:
			if err != nil {
				select {
				case <-ctx.Done():
					server.err = ctx.Err()
					return
				default:
				}
			}
			if err != nil && !errors.Is(err, context.Canceled) {
				server.err = fmt.Errorf("run harness: %w", err)
			}
		case <-ctx.Done():
			server.err = ctx.Err()
		}
	}()

	return server, nil
}

// Address returns the bound TCP address.
func (s *Server) Address() string {
	return s.Addr
}

func (s *Server) Wait() error {
	<-s.done

	return s.err
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.terminal.Shutdown()

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
