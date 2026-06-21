package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/kurtisvg/ahh/internal/harness"
	"github.com/kurtisvg/ahh/internal/logging"
	webassets "github.com/kurtisvg/ahh/internal/web"
)

// HarnessSupervisor is the server-owned harness lifecycle boundary.
type HarnessSupervisor interface {
	Start(context.Context) error
	Stop(context.Context) error
	Status() harness.Status
}

// Options configures the Ahh HTTP server.
type Options struct {
	HarnessSupervisor HarnessSupervisor
}

// NewHandler builds the HTTP routes for the local Ahh server.
func NewHandler() http.Handler {
	return NewHandlerWithOptions(Options{})
}

// NewHandlerWithOptions builds the HTTP routes for the local Ahh server.
func NewHandlerWithOptions(opts Options) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", index)
	mux.HandleFunc("GET /healthz", healthz)
	if opts.HarnessSupervisor != nil {
		mux.HandleFunc("GET /api/harness/status", harnessStatus(opts.HarnessSupervisor))
	}
	mux.Handle("GET /static/", noStore(http.StripPrefix("/static/", http.FileServer(http.FS(webassets.Files)))))
	return mux
}

// Listen opens a TCP listener for the configured HTTP host and port.
func Listen(host, port string) (net.Listener, error) {
	addr := net.JoinHostPort(host, port)
	return net.Listen("tcp", addr)
}

// Serve runs the HTTP server until the context is canceled or serving fails.
func Serve(ctx context.Context, ln net.Listener) error {
	return ServeWithOptions(ctx, ln, Options{})
}

// ServeWithOptions runs the HTTP server and configured harness supervision.
func ServeWithOptions(ctx context.Context, ln net.Listener, opts Options) error {
	logger := logging.FromContext(ctx)
	if opts.HarnessSupervisor != nil {
		if err := opts.HarnessSupervisor.Start(ctx); err != nil {
			return err
		}
		defer func() {
			stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := opts.HarnessSupervisor.Stop(stopCtx); err != nil {
				logger.Warn("harness shutdown error", "error", err)
			}
		}()
		logger.Info("harness ready", "status", opts.HarnessSupervisor.Status())
	}

	srv := &http.Server{
		Handler:           NewHandlerWithOptions(opts),
		ReadHeaderTimeout: 30 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Warn("http server shutdown error", "error", err)
		}
	}()

	logger.Info("listening", "addr", ln.Addr().String())
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// healthz reports whether the local server can answer requests.
func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func harnessStatus(supervisor HarnessSupervisor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(supervisor.Status()); err != nil {
			logging.FromContext(r.Context()).Warn("encode harness status error", "error", err)
		}
	}
}

// noStore prevents development static assets from being cached by browsers or proxies.
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Static assets change often during early UI work, and proxied views may cache aggressively.
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// index serves the browser shell at the root route.
func index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	indexHTML, err := webassets.Files.ReadFile("index.html")
	if err != nil {
		http.Error(w, "failed to read index", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(indexHTML)
}
