package middleware

import (
	"context"

	"go.opentelemetry.io/otel/metric"
)

type logger interface {
	InfoContext(ctx context.Context, msg string, args ...any)
	ErrorContext(ctx context.Context, msg string, args ...any)
}

type upDownCounter interface {
	Add(ctx context.Context, incr int64, options ...metric.AddOption)
}
