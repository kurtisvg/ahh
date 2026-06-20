package wrapper

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/kurtisvg/ahh/internal/logging"
)

// NewHandler builds the HTTP routes for a harness wrapper process.
func NewHandler(harness string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	mux.HandleFunc("GET /readyz", readyz(harness))
	return mux
}

// Listen opens a TCP listener for the wrapper HTTP server.
func Listen(host, port string) (net.Listener, error) {
	addr := net.JoinHostPort(host, port)
	return net.Listen("tcp", addr)
}

// Serve runs the wrapper HTTP server until the context is canceled or serving fails.
func Serve(ctx context.Context, ln net.Listener, harness string) error {
	logger := logging.FromContext(ctx)
	srv := &http.Server{
		Handler:           NewHandler(harness),
		ReadHeaderTimeout: 30 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Warn("wrapper server shutdown error", "error", err)
		}
	}()

	logger.Info("wrapper listening", "harness", harness, "addr", ln.Addr().String())
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func readyz(harness string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ready":true,"harness":"` + harness + `"}` + "\n"))
	}
}
