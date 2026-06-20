package logging

import (
	"context"
	"log/slog"
)

type loggerContextKey struct{}

// WithLogger returns a context carrying logger, defaulting to slog.Default when nil.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if logger == nil {
		logger = slog.Default()
	}
	return context.WithValue(ctx, loggerContextKey{}, logger)
}

// FromContext returns the logger stored on ctx, or slog.Default when none exists.
func FromContext(ctx context.Context) *slog.Logger {
	logger, ok := ctx.Value(loggerContextKey{}).(*slog.Logger)
	if !ok || logger == nil {
		return slog.Default()
	}
	return logger
}
