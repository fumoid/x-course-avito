package postgres

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const (
	attrDBOperation  = "db.operation.name"
	attrDBCollection = "db.collection.name"
	attrErrorType    = "error.type"
	unknownSQL       = "UNKNOWN"
	sqlTransaction   = "transaction"
)

type querySpanKey struct{}

type querySpan struct {
	span      trace.Span
	start     time.Time
	operation string
	table     string
}

type QueryTracer struct {
	tracer    trace.Tracer
	histogram metric.Float64Histogram
}

func NewQueryTracer(tracer trace.Tracer, histogram metric.Float64Histogram) *QueryTracer {
	return &QueryTracer{tracer: tracer, histogram: histogram}
}

func (t *QueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if t == nil {
		return ctx
	}
	operation, table := parseSQL(data.SQL)
	info := querySpan{
		start:     time.Now(),
		operation: operation,
		table:     table,
	}
	if t.tracer != nil && trace.SpanFromContext(ctx).SpanContext().IsValid() {
		var span trace.Span
		ctx, span = t.tracer.Start(ctx, operation+" "+table,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				attribute.String(attrDBOperation, operation),
				attribute.String(attrDBCollection, table),
				attribute.String("db.system", "postgresql"),
			),
		)
		info.span = span
	}
	return context.WithValue(ctx, querySpanKey{}, info)
}

func (t *QueryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	if t == nil {
		return
	}
	info, ok := ctx.Value(querySpanKey{}).(querySpan)
	if !ok {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String(attrDBOperation, info.operation),
		attribute.String(attrDBCollection, info.table),
	}
	if data.Err != nil {
		errType := errorType(data.Err)
		attrs = append(attrs, attribute.String(attrErrorType, errType))
		if info.span != nil {
			info.span.SetStatus(codes.Error, data.Err.Error())
			info.span.SetAttributes(attribute.String(attrErrorType, errType))
		}
	}
	if info.span != nil {
		info.span.End()
	}
	if t.histogram != nil {
		t.histogram.Record(ctx, time.Since(info.start).Seconds(), metric.WithAttributes(attrs...))
	}
}

func parseSQL(sql string) (string, string) {
	trimmed := strings.TrimSpace(sql)
	if trimmed == "" {
		return unknownSQL, unknownSQL
	}
	upper := strings.ToUpper(trimmed)
	switch {
	case strings.Contains(upper, "PG_ADVISORY_XACT_LOCK"):
		return "SELECT", "idempotency_keys"
	case strings.HasPrefix(upper, "INSERT"):
		return "INSERT", tableAfter(upper, "INTO")
	case strings.HasPrefix(upper, "UPDATE"):
		return "UPDATE", firstIdent(strings.TrimSpace(upper[len("UPDATE"):]))
	case strings.HasPrefix(upper, "DELETE"):
		return "DELETE", tableAfter(upper, "FROM")
	case strings.HasPrefix(upper, "SELECT"):
		return "SELECT", tableAfter(upper, "FROM")
	case strings.HasPrefix(upper, "BEGIN"):
		return "BEGIN", sqlTransaction
	case strings.HasPrefix(upper, "COMMIT"):
		return "COMMIT", sqlTransaction
	case strings.HasPrefix(upper, "ROLLBACK"):
		return "ROLLBACK", sqlTransaction
	default:
		return unknownSQL, unknownSQL
	}
}

func tableAfter(upper, keyword string) string {
	idx := strings.Index(upper, " "+keyword+" ")
	if idx < 0 {
		idx = strings.Index(upper, keyword+" ")
		if idx != 0 && idx != 1 {
			return unknownSQL
		}
	}
	rest := strings.TrimSpace(upper[idx+len(keyword)+1:])
	return firstIdent(rest)
}

func firstIdent(s string) string {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	s = strings.Trim(s, `"`)
	end := 0
	for end < len(s) {
		c := rune(s[end])
		if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' {
			end++
			continue
		}
		break
	}
	if end == 0 {
		return unknownSQL
	}
	return strings.ToLower(s[:end])
}

func errorType(err error) string {
	if err == nil {
		return ""
	}
	var target interface{ SQLState() string }
	if errors.As(err, &target) && target.SQLState() != "" {
		return target.SQLState()
	}
	return "_OTHER"
}
