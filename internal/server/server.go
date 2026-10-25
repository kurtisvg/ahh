// Package server implements the orchestrator HTTP service.
package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/kurtisvg/ahh/internal/project"
	"github.com/kurtisvg/ahh/internal/project/local"
	"github.com/kurtisvg/ahh/internal/store"
)

// Server owns the workspace lock, HTTP, storage, wrapper, and terminal proxies.
// Create one with Start. Wait joins serving and all owned resources.
type Server struct {
	Address   string
	Port      int
	Workspace string // Canonical workspace path.

	http           *http.Server
	store          store.Store
	projects       project.Manager
	workspaceLock  *os.File
	wrapperMu      sync.Mutex // Serializes launch and shutdown; proxies snapshot the process.
	wrapper        *wrapperProcess
	launch         func() (*wrapperProcess, error)
	cancelRequests context.CancelFunc
	requestsMu     sync.Mutex // Prevents registration after shutdown starts.
	requests       sync.WaitGroup

	done chan struct{}
	err  error // Read after done closes.
}

// Start locks the workspace, opens storage, and binds HTTP without a harness. POST /start
// launches wrap codex in the current directory. Port zero selects a free port.
// Cancel ctx or call Shutdown to stop it. Failed startup releases all resources.
// Workspace ownership is released before Wait returns.
func Start(ctx context.Context, config Config) (*Server, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate executable: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	lock, err := lockWorkspace(config.Workspace)
	if err != nil {
		return nil, err
	}
	config.Workspace = filepath.Dir(lock.Name())
	if config.Store.Backend == store.SQLite && config.Store.Path != "" && !filepath.IsAbs(config.Store.Path) {
		config.Store.Path = filepath.Join(config.Workspace, config.Store.Path)
	}
	storage, err := openStore(ctx, config.Store)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("open store: %w", err), lock.Close())
	}
	args := []string{"wrap", "codex", "--address", "127.0.0.1", "--port", "0"}
	return start(ctx, config, storage, lock, executable, args...)
}

// start takes ownership of the required store and lock, including on failure.
func start(
	ctx context.Context,
	config Config,
	storage store.Store,
	lock *os.File,
	executable string,
	args ...string,
) (srv *Server, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(err, storage.Close(), lock.Close())
		}
	}()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	basePath, err := normalizeBasePath(config.BasePath)
	if err != nil {
		return nil, err
	}
	page, err := servePage(basePath)
	if err != nil {
		return nil, fmt.Errorf("prepare page: %w", err)
	}
	projects, err := local.New(ctx, filepath.Dir(lock.Name()), storage)
	if err != nil {
		return nil, fmt.Errorf("open project manager: %w", err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, projects.Close())
		}
	}()

	listenAddress := net.JoinHostPort(config.Address, strconv.Itoa(config.Port))
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", listenAddress)
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

	requestCtx, cancelRequests := context.WithCancel(ctx)
	protection := http.NewCrossOriginProtection()
	mux := http.NewServeMux()
	mux.Handle("GET /", serveAssets())
	mux.Handle("GET /{$}", page)
	mux.Handle("GET /terminal", page)
	mux.Handle("GET /p", page)
	mux.Handle("GET /p/{id}", page)
	server := &Server{
		Address:       actualAddress,
		Port:          actualPort,
		Workspace:     filepath.Dir(lock.Name()),
		store:         storage,
		projects:      projects,
		workspaceLock: lock,
		http: &http.Server{
			ReadHeaderTimeout: 5 * time.Second,
			BaseContext:       func(net.Listener) context.Context { return requestCtx },
		},
		launch: func() (*wrapperProcess, error) {
			if err := requestCtx.Err(); err != nil {
				return nil, err
			}
			return startWrapper(requestCtx, executable, args...)
		},
		cancelRequests: cancelRequests,
		done:           make(chan struct{}),
	}
	mux.HandleFunc("GET /api/projects", server.listProjects)
	mux.HandleFunc("POST /api/projects", server.createProject)
	mux.HandleFunc("PATCH /api/projects/{id}", server.renameProject)
	mux.HandleFunc("GET /api/projects/{id}", server.getProject)
	mux.HandleFunc("GET /pty", server.proxyPTY(protection))
	mux.HandleFunc("GET /status", server.serveStatus)
	mux.HandleFunc("POST /start", server.serveStart)
	server.http.Handler = protection.Handler(server.trackRequests(requestCtx, mux))
	go server.serve(ctx, listener)
	return server, nil
}

func (s *Server) serve(ctx context.Context, listener net.Listener) {
	defer close(s.done)
	defer func() {
		s.stopRequests()
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		s.wrapperMu.Lock()
		if s.wrapper != nil {
			_ = s.wrapper.Shutdown(shutdown)
			_ = s.wrapper.Wait()
		}
		s.wrapperMu.Unlock()
		s.requests.Wait()
		s.err = errors.Join(s.err, s.projects.Close(), s.store.Close(), s.workspaceLock.Close())
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

// Wait joins serving, handlers, and wrapper cleanup, then closes storage and
// releases the workspace lock. It reports serving and cleanup errors.
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

// Count all handlers so even a forced HTTP shutdown leaves storage open until
// requests finish. The mutex prevents registration after shutdown starts.
func (s *Server) trackRequests(ctx context.Context, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requestsMu.Lock()
		if ctx.Err() != nil {
			s.requestsMu.Unlock()
			http.Error(w, "server is shutting down", http.StatusServiceUnavailable)
			return
		}
		s.requests.Add(1)
		s.requestsMu.Unlock()
		defer s.requests.Done()
		next.ServeHTTP(w, r)
	})
}
