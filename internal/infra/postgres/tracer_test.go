package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

func TestParseSQL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		sql       string
		operation string
		table     string
	}{
		{sql: "INSERT INTO trips (id) VALUES ($1)", operation: "INSERT", table: "trips"},
		{sql: "SELECT id FROM trips WHERE id = $1", operation: "SELECT", table: "trips"},
		{sql: "UPDATE trips SET status = $1 WHERE id = $2", operation: "UPDATE", table: "trips"},
		{sql: "DELETE FROM idempotency_keys WHERE key = $1", operation: "DELETE", table: "idempotency_keys"},
		{sql: "SELECT pg_advisory_xact_lock(hashtext($1))", operation: "SELECT", table: "idempotency_keys"},
		{sql: "INSERT INTO trip_status_history (trip_id) VALUES ($1)", operation: "INSERT", table: "trip_status_history"},
		{sql: "BEGIN", operation: "BEGIN", table: "transaction"},
		{sql: "COMMIT", operation: "COMMIT", table: "transaction"},
		{sql: "ROLLBACK", operation: "ROLLBACK", table: "transaction"},
		{sql: "", operation: unknownSQL, table: unknownSQL},
	}
	for _, tc := range cases {
		t.Run(tc.sql, func(t *testing.T) {
			t.Parallel()
			op, table := parseSQL(tc.sql)
			if op != tc.operation || table != tc.table {
				t.Fatalf("got %s %s, want %s %s", op, table, tc.operation, tc.table)
			}
		})
	}
}

func TestErrorType(t *testing.T) {
	t.Parallel()
	if got := errorType(errors.New("dial tcp: connection refused")); got != "_OTHER" {
		t.Fatalf("got %q", got)
	}
}

type recordHistogram struct {
	metric.Float64Histogram
	n int
}

func (h *recordHistogram) Record(context.Context, float64, ...metric.RecordOption) {
	h.n++
}

func TestQueryTracerRecordsDurationWithoutParentSpan(t *testing.T) {
	t.Parallel()
	hist := &recordHistogram{Float64Histogram: metricnoop.Float64Histogram{}}
	tracer := NewQueryTracer(tracenoop.NewTracerProvider().Tracer("test"), hist)
	ctx := tracer.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: "SELECT id FROM trips"})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})
	if hist.n != 1 {
		t.Fatalf("histogram records = %d, want 1", hist.n)
	}
}

func TestQueryTracerRecordsDurationWhenParentNotRecording(t *testing.T) {
	t.Parallel()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.NeverSample()))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	parentCtx, parent := tp.Tracer("http").Start(context.Background(), "POST /api/v1/trips")
	t.Cleanup(func() { parent.End() })
	if parent.IsRecording() {
		t.Fatal("parent should not be recording")
	}
	if !parent.SpanContext().IsValid() {
		t.Fatal("parent span context should be valid")
	}

	hist := &recordHistogram{Float64Histogram: metricnoop.Float64Histogram{}}
	tracer := NewQueryTracer(tp.Tracer("db"), hist)
	ctx := tracer.TraceQueryStart(parentCtx, nil, pgx.TraceQueryStartData{SQL: "SELECT id FROM trips"})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})
	if hist.n != 1 {
		t.Fatalf("histogram records = %d, want 1", hist.n)
	}
}
