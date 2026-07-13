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

const (
	ClaudeCodeHarness = "claude-code"

	shutdownTimeout = 5 * time.Second
)

// Wrapper manages a running harness wrapper.
type Wrapper interface {
	Address() string
	SessionID() string
	Wait() error
	Shutdown(context.Context) error
}

type options struct {
	sessionID string
	resume    bool
}

// Option configures a wrapper and its harness process.
type Option func(*options) error

// WithSessionID uses id for a new harness session instead of generating one.
func WithSessionID(id string) Option {
	return func(opts *options) error {
		return configureSession(opts, id, false)
	}
}

// WithResume configures the harness to resume the session with id.
func WithResume(id string) Option {
	return func(opts *options) error {
		return configureSession(opts, id, true)
	}
}

func configureSession(opts *options, id string, resume bool) error {
	parsed, err := uuid.Parse(id)
	if err != nil || parsed == uuid.Nil {
		return fmt.Errorf("invalid session id %q", id)
	}
	if opts.sessionID != "" {
		return fmt.Errorf("session id is already configured")
	}
	opts.sessionID = parsed.String()
	opts.resume = resume
	return nil
}

// Server exposes the wrapper API for a running harness.
type Server struct {
	Addr       string
	sessionID  string
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
func Start(ctx context.Context, harnessName string, addr string, opts ...Option) (*Server, error) {
	cfg, err := resolveOptions(opts...)
	if err != nil {
		return nil, err
	}

	h, err := startHarness(ctx, harnessName, cfg.sessionID, cfg.resume)
	if err != nil {
		return nil, err
	}

	server, err := start(ctx, h, addr, cfg.sessionID)
	if err != nil {
		h.Close()
		return nil, err
	}

	return server, nil
}

func resolveOptions(opts ...Option) (options, error) {
	cfg := options{}
	for _, opt := range opts {
		if err := opt(&cfg); err != nil {
			return options{}, err
		}
	}
	if cfg.sessionID != "" {
		return cfg, nil
	}

	id, err := newSessionID()
	if err != nil {
		return options{}, fmt.Errorf("generate session id: %w", err)
	}
	cfg.sessionID = id

	return cfg, nil
}

func newSessionID() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}

	return id.String(), nil
}

func startHarness(
	ctx context.Context,
	harnessName string,
	sessionID string,
	resume bool,
) (harness.Harness, error) {
	switch harnessName {
	case ClaudeCodeHarness:
		var opt harness.Option
		if resume {
			opt = harness.WithResume(sessionID)
		} else {
			opt = harness.WithSessionID(sessionID)
		}
		h, err := harness.Start(ctx, opt)
		if err != nil {
			return nil, fmt.Errorf("start %s harness: %w", harnessName, err)
		}

		return h, nil
	default:
		return nil, fmt.Errorf("unsupported harness %q", harnessName)
	}
}

func start(ctx context.Context, h harness.Harness, addr, sessionID string) (*Server, error) {
	if addr == "" {
		return nil, fmt.Errorf("wrapper address is required")
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}

	server := &Server{
		Addr:      listener.Addr().String(),
		sessionID: sessionID,
		harness:   h,
		terminal:  newTerminalSession(h),
		done:      make(chan struct{}),
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

// SessionID returns the harness session identity assigned at startup.
func (s *Server) SessionID() string {
	return s.sessionID
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
