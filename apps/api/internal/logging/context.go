package logging

import (
	"context"
	"log/slog"
)

type loggerKey struct{}

// FromContext returns the request-scoped logger, falling back to
// slog.Default if none was attached.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// WithLogger returns a derived context carrying logger.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// RequestLog holds per-request mutable log fields. The slog middleware
// seeds one via WithRequestLog; downstream middleware enriches it.
type RequestLog struct {
	Workspace   string
	WorkspaceID string
}

type requestLogKey struct{}

// GetRequestLog returns the per-request log struct, or nil.
func GetRequestLog(ctx context.Context) *RequestLog {
	if rl, ok := ctx.Value(requestLogKey{}).(*RequestLog); ok {
		return rl
	}
	return nil
}

// WithRequestLog returns a context carrying a mutable RequestLog pointer.
func WithRequestLog(ctx context.Context, rl *RequestLog) context.Context {
	return context.WithValue(ctx, requestLogKey{}, rl)
}
