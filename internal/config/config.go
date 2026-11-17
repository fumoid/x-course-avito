package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHTTPAddr              = ":8080"
	defaultLogLevel              = "info"
	defaultShutdownTimeout       = "10s"
	defaultHTTPReadTimeout       = "10s"
	defaultHTTPReadHeaderTimeout = "5s"
	defaultHTTPWriteTimeout      = "15s"
	defaultHTTPIdleTimeout       = "60s"
	defaultDatabaseMaxConns      = "10"
	defaultDatabaseMinConns      = "2"
	defaultDatabaseMaxLifetime   = "30m"
	defaultDatabaseConnect       = "5s"
	defaultDatabaseQuery         = "3s"
	defaultIdempotencyTTL        = "24h"
	defaultPositionCheckInterval = "30s"
	defaultPositionStaleAfter    = "60s"
	defaultPositionWorkers       = "4"
	defaultPositionQueueSize     = "100"
	defaultPositionScanBatchSize = "100"
	defaultPushHTTPURL           = "http://localhost:8090"
	defaultPushRequestTimeout    = "1s"
	defaultOTELServiceName       = "trip-service"
	defaultOTELProtocol          = "grpc"
	defaultOTELSampler           = "parentbased_always_on"
	defaultOTELResourceAttrs     = "service.namespace=tripgo,deployment.environment=local"
	otelSemConvStabilityOptIn    = "OTEL_SEMCONV_STABILITY_OPT_IN"
	defaultOTELSemConvOptIn      = "http/dup"
)

type Config struct {
	HTTPAddr                 string
	LogLevel                 string
	ShutdownTimeout          time.Duration
	HTTPReadTimeout          time.Duration
	HTTPReadHeaderTimeout    time.Duration
	HTTPWriteTimeout         time.Duration
	HTTPIdleTimeout          time.Duration
	DatabaseURL              string
	DatabaseMaxConns         int32
	DatabaseMinConns         int32
	DatabaseMaxConnLifetime  time.Duration
	DatabaseConnectTimeout   time.Duration
	DatabaseQueryTimeout     time.Duration
	IdempotencyTTL           time.Duration
	PositionCheckInterval    time.Duration
	PositionStaleAfter       time.Duration
	PositionWorkers          int
	PositionQueueSize        int
	PositionScanBatchSize    int
	PushHTTPURL              string
	PushRequestTimeout       time.Duration
	OTELServiceName          string
	OTELExporterOTLPEndpoint string
	OTELExporterOTLPProtocol string
	OTELTracesSampler        string
	OTELTracesSamplerArg     string
	OTELResourceAttributes   string
}

func Load() (Config, error) {
	databaseURL, err := requiredString("DATABASE_URL")
	if err != nil {
		return Config{}, err
	}

	httpAddr := envOrDefault("HTTP_ADDR", defaultHTTPAddr)
	logLevel := strings.ToLower(envOrDefault("LOG_LEVEL", defaultLogLevel))
	if err := validateLogLevel(logLevel); err != nil {
		return Config{}, err
	}

	shutdownTimeout, err := positiveDuration("SHUTDOWN_TIMEOUT", defaultShutdownTimeout)
	if err != nil {
		return Config{}, err
	}
	readTimeout, err := positiveDuration("HTTP_READ_TIMEOUT", defaultHTTPReadTimeout)
	if err != nil {
		return Config{}, err
	}
	readHeaderTimeout, err := positiveDuration("HTTP_READ_HEADER_TIMEOUT", defaultHTTPReadHeaderTimeout)
	if err != nil {
		return Config{}, err
	}
	writeTimeout, err := positiveDuration("HTTP_WRITE_TIMEOUT", defaultHTTPWriteTimeout)
	if err != nil {
		return Config{}, err
	}
	idleTimeout, err := positiveDuration("HTTP_IDLE_TIMEOUT", defaultHTTPIdleTimeout)
	if err != nil {
		return Config{}, err
	}
	lifetime, err := positiveDuration("DATABASE_MAX_CONN_LIFETIME", defaultDatabaseMaxLifetime)
	if err != nil {
		return Config{}, err
	}
	connectTimeout, err := positiveDuration("DATABASE_CONNECT_TIMEOUT", defaultDatabaseConnect)
	if err != nil {
		return Config{}, err
	}
	queryTimeout, err := positiveDuration("DATABASE_QUERY_TIMEOUT", defaultDatabaseQuery)
	if err != nil {
		return Config{}, err
	}
	ttl, err := positiveDuration("IDEMPOTENCY_TTL", defaultIdempotencyTTL)
	if err != nil {
		return Config{}, err
	}
	checkInterval, err := positiveDuration("POSITION_CHECK_INTERVAL", defaultPositionCheckInterval)
	if err != nil {
		return Config{}, err
	}
	staleAfter, err := positiveDuration("POSITION_STALE_AFTER", defaultPositionStaleAfter)
	if err != nil {
		return Config{}, err
	}
	pushTimeout, err := positiveDuration("PUSH_REQUEST_TIMEOUT", defaultPushRequestTimeout)
	if err != nil {
		return Config{}, err
	}
	workers, err := positiveInt("POSITION_WORKERS", defaultPositionWorkers)
	if err != nil {
		return Config{}, err
	}
	queueSize, err := positiveInt("POSITION_QUEUE_SIZE", defaultPositionQueueSize)
	if err != nil {
		return Config{}, err
	}
	scanBatch, err := positiveInt("POSITION_SCAN_BATCH_SIZE", defaultPositionScanBatchSize)
	if err != nil {
		return Config{}, err
	}

	maxConns, err := int32Env("DATABASE_MAX_CONNS", defaultDatabaseMaxConns, 1)
	if err != nil {
		return Config{}, err
	}
	minConns, err := int32Env("DATABASE_MIN_CONNS", defaultDatabaseMinConns, 0)
	if err != nil {
		return Config{}, err
	}
	if minConns > maxConns {
		return Config{}, fmt.Errorf("DATABASE_MIN_CONNS must be less than or equal to DATABASE_MAX_CONNS")
	}

	enableHTTPRequestDuration()

	return Config{
		HTTPAddr:                 httpAddr,
		LogLevel:                 logLevel,
		ShutdownTimeout:          shutdownTimeout,
		HTTPReadTimeout:          readTimeout,
		HTTPReadHeaderTimeout:    readHeaderTimeout,
		HTTPWriteTimeout:         writeTimeout,
		HTTPIdleTimeout:          idleTimeout,
		DatabaseURL:              databaseURL,
		DatabaseMaxConns:         maxConns,
		DatabaseMinConns:         minConns,
		DatabaseMaxConnLifetime:  lifetime,
		DatabaseConnectTimeout:   connectTimeout,
		DatabaseQueryTimeout:     queryTimeout,
		IdempotencyTTL:           ttl,
		PositionCheckInterval:    checkInterval,
		PositionStaleAfter:       staleAfter,
		PositionWorkers:          workers,
		PositionQueueSize:        queueSize,
		PositionScanBatchSize:    scanBatch,
		PushHTTPURL:              envOrDefault("PUSH_HTTP_URL", defaultPushHTTPURL),
		PushRequestTimeout:       pushTimeout,
		OTELServiceName:          envOrDefault("OTEL_SERVICE_NAME", defaultOTELServiceName),
		OTELExporterOTLPEndpoint: envOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		OTELExporterOTLPProtocol: strings.ToLower(envOrDefault("OTEL_EXPORTER_OTLP_PROTOCOL", defaultOTELProtocol)),
		OTELTracesSampler:        strings.ToLower(envOrDefault("OTEL_TRACES_SAMPLER", defaultOTELSampler)),
		OTELTracesSamplerArg:     envOrDefault("OTEL_TRACES_SAMPLER_ARG", ""),
		OTELResourceAttributes:   envOrDefault("OTEL_RESOURCE_ATTRIBUTES", defaultOTELResourceAttrs),
	}, nil
}

func requiredString(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func envOrDefault(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func positiveDuration(name, fallback string) (time.Duration, error) {
	raw := envOrDefault(name, fallback)
	duration, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration: %w", name, err)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", name)
	}
	return duration, nil
}

func positiveInt(name, fallback string) (int, error) {
	value, err := int32Env(name, fallback, 1)
	if err != nil {
		return 0, err
	}
	return int(value), nil
}

func int32Env(name, fallback string, minimum int64) (int32, error) {
	raw := envOrDefault(name, fallback)
	parsed, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid integer: %w", name, err)
	}
	if parsed < minimum {
		return 0, fmt.Errorf("%s must be greater than or equal to %d", name, minimum)
	}
	return int32(parsed), nil
}

func enableHTTPRequestDuration() {
	if strings.TrimSpace(os.Getenv(otelSemConvStabilityOptIn)) == "" {
		_ = os.Setenv(otelSemConvStabilityOptIn, defaultOTELSemConvOptIn)
	}
}

func validateLogLevel(level string) error {
	switch level {
	case "debug", "info", "warn", "error":
		return nil
	default:
		return fmt.Errorf("LOG_LEVEL must be one of: debug, info, warn, error")
	}
}
