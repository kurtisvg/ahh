package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	webassets "github.com/kurtisvg/ahh/internal/web"
)

func NewHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", index)
	mux.HandleFunc("GET /healthz", healthz)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(webassets.Files))))
	return mux
}

func Listen(host, port string) (net.Listener, error) {
	addr := net.JoinHostPort(host, port)
	return net.Listen("tcp", addr)
}

func Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler:           NewHandler(),
		ReadHeaderTimeout: 30 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Warn("http server shutdown error", "error", err)
		}
	}()

	slog.Info("listening", "addr", ln.Addr().String())
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

func index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFileFS(w, r, webassets.Files, "index.html")
}
