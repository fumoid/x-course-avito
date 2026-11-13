package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/fumoid/x-course-avito/internal/config"
)

const (
	serviceVersion = "lab2"
	meterName      = "github.com/fumoid/x-course-avito"
	tracerName     = "github.com/fumoid/x-course-avito"
)

type Provider struct {
	Logger         *slog.Logger
	Tracer         trace.Tracer
	Meter          metric.Meter
	TripMetrics    *TripMetrics
	ActiveRequests metric.Int64UpDownCounter
	DBDuration     metric.Float64Histogram
	shutdown       []func(context.Context) error
}

type TripMetrics struct {
	created   metric.Int64Counter
	completed metric.Int64Counter
}

func (m *TripMetrics) TripCreated(ctx context.Context) {
	if m == nil || m.created == nil {
		return
	}
	m.created.Add(ctx, 1)
}

func (m *TripMetrics) TripCompleted(ctx context.Context) {
	if m == nil || m.completed == nil {
		return
	}
	m.completed.Add(ctx, 1)
}

func New(ctx context.Context, cfg config.Config) (*Provider, error) {
	logger := newLogger(cfg)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.Error("otel export failed", "error", err)
	}))

	res, err := newResource(cfg)
	if err != nil {
		return nil, fmt.Errorf("build otel resource: %w", err)
	}

	provider := &Provider{Logger: logger}

	traceExporter, err := newTraceExporter(ctx, cfg)
	if err != nil {
		logger.ErrorContext(ctx, "otel trace exporter init failed, continuing without traces", "error", err)
	} else {
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(traceExporter),
			sdktrace.WithResource(res),
			sdktrace.WithSampler(samplerFromEnv(cfg)),
		)
		otel.SetTracerProvider(tp)
		provider.shutdown = append(provider.shutdown, tp.Shutdown)
	}

	metricExporter, err := newMetricExporter(ctx, cfg)
	if err != nil {
		logger.ErrorContext(ctx, "otel metric exporter init failed, continuing without metrics", "error", err)
	} else {
		mp := sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(15*time.Second))),
			sdkmetric.WithResource(res),
		)
		otel.SetMeterProvider(mp)
		provider.shutdown = append(provider.shutdown, mp.Shutdown)
	}

	provider.Tracer = otel.Tracer(tracerName)
	provider.Meter = otel.Meter(meterName)
	if err := provider.initInstruments(); err != nil {
		return nil, err
	}
	return provider, nil
}

func (p *Provider) initInstruments() error {
	created, err := p.Meter.Int64Counter("tripgo.trips.created",
		metric.WithDescription("created trips"),
		metric.WithUnit("{trip}"),
	)
	if err != nil {
		return fmt.Errorf("create trips.created counter: %w", err)
	}
	completed, err := p.Meter.Int64Counter("tripgo.trips.completed",
		metric.WithDescription("completed trips"),
		metric.WithUnit("{trip}"),
	)
	if err != nil {
		return fmt.Errorf("create trips.completed counter: %w", err)
	}
	p.TripMetrics = &TripMetrics{created: created, completed: completed}

	active, err := p.Meter.Int64UpDownCounter("http.server.active_requests",
		metric.WithDescription("in-flight HTTP requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return fmt.Errorf("create active_requests counter: %w", err)
	}
	p.ActiveRequests = active

	dbDuration, err := p.Meter.Float64Histogram("db.client.operation.duration",
		metric.WithDescription("database operation duration"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return fmt.Errorf("create db duration histogram: %w", err)
	}
	p.DBDuration = dbDuration
	return nil
}

func (p *Provider) RegisterActiveTrips(count func(context.Context) (int64, error)) error {
	_, err := p.Meter.Int64ObservableGauge("tripgo.trips.active",
		metric.WithDescription("trips currently in status active"),
		metric.WithUnit("{trip}"),
		metric.WithInt64Callback(func(ctx context.Context, observer metric.Int64Observer) error {
			n, err := count(ctx)
			if err != nil {
				return err
			}
			observer.Observe(n)
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("create trips.active gauge: %w", err)
	}
	return nil
}

func (p *Provider) Shutdown(ctx context.Context) error {
	var first error
	for i := len(p.shutdown) - 1; i >= 0; i-- {
		if err := p.shutdown[i](ctx); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func newLogger(cfg config.Config) *slog.Logger {
	var level slog.Level
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(newTraceHandler(handler)).With("service", cfg.OTELServiceName)
}

func newResource(cfg config.Config) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(cfg.OTELServiceName),
		semconv.ServiceVersion(serviceVersion),
	}
	for _, pair := range strings.Split(cfg.OTELResourceAttributes, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		attrs = append(attrs, attribute.String(strings.TrimSpace(key), strings.TrimSpace(value)))
	}
	return resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL, attrs...),
	)
}

func samplerFromEnv(cfg config.Config) sdktrace.Sampler {
	arg := cfg.OTELTracesSamplerArg
	switch cfg.OTELTracesSampler {
	case "always_on":
		return sdktrace.AlwaysSample()
	case "always_off":
		return sdktrace.NeverSample()
	case "traceidratio":
		return sdktrace.TraceIDRatioBased(parseRatio(arg))
	case "parentbased_always_off":
		return sdktrace.ParentBased(sdktrace.NeverSample())
	case "parentbased_traceidratio":
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(parseRatio(arg)))
	default:
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	}
}

func parseRatio(raw string) float64 {
	if raw == "" {
		return 1
	}
	ratio, err := strconv.ParseFloat(raw, 64)
	if err != nil || ratio < 0 {
		return 1
	}
	if ratio > 1 {
		return 1
	}
	return ratio
}

func newTraceExporter(ctx context.Context, cfg config.Config) (sdktrace.SpanExporter, error) {
	if !httpProtocol(cfg.OTELExporterOTLPProtocol) {
		opts := []otlptracegrpc.Option{otlptracegrpc.WithInsecure()}
		if cfg.OTELExporterOTLPEndpoint != "" {
			opts = append(opts, otlptracegrpc.WithEndpoint(grpcEndpoint(cfg.OTELExporterOTLPEndpoint)))
		}
		return otlptracegrpc.New(ctx, opts...)
	}
	opts := []otlptracehttp.Option{otlptracehttp.WithInsecure()}
	if cfg.OTELExporterOTLPEndpoint != "" {
		opts = append(opts, otlptracehttp.WithEndpoint(httpEndpoint(cfg.OTELExporterOTLPEndpoint)))
	}
	return otlptracehttp.New(ctx, opts...)
}

func newMetricExporter(ctx context.Context, cfg config.Config) (sdkmetric.Exporter, error) {
	if !httpProtocol(cfg.OTELExporterOTLPProtocol) {
		opts := []otlpmetricgrpc.Option{otlpmetricgrpc.WithInsecure()}
		if cfg.OTELExporterOTLPEndpoint != "" {
			opts = append(opts, otlpmetricgrpc.WithEndpoint(grpcEndpoint(cfg.OTELExporterOTLPEndpoint)))
		}
		return otlpmetricgrpc.New(ctx, opts...)
	}
	opts := []otlpmetrichttp.Option{otlpmetrichttp.WithInsecure()}
	if cfg.OTELExporterOTLPEndpoint != "" {
		opts = append(opts, otlpmetrichttp.WithEndpoint(httpEndpoint(cfg.OTELExporterOTLPEndpoint)))
	}
	return otlpmetrichttp.New(ctx, opts...)
}

func httpProtocol(protocol string) bool {
	return protocol == "http/protobuf" || protocol == "http" || protocol == "http/json"
}

func grpcEndpoint(raw string) string {
	raw = strings.TrimPrefix(raw, "http://")
	raw = strings.TrimPrefix(raw, "https://")
	return raw
}

func httpEndpoint(raw string) string {
	return grpcEndpoint(raw)
}
