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
)

type Config struct {
	HTTPAddr                string
	LogLevel                string
	ShutdownTimeout         time.Duration
	HTTPReadTimeout         time.Duration
	HTTPReadHeaderTimeout   time.Duration
	HTTPWriteTimeout        time.Duration
	HTTPIdleTimeout         time.Duration
	DatabaseURL             string
	DatabaseMaxConns        int32
	DatabaseMinConns        int32
	DatabaseMaxConnLifetime time.Duration
	DatabaseConnectTimeout  time.Duration
	DatabaseQueryTimeout    time.Duration
	IdempotencyTTL          time.Duration
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

	return Config{
		HTTPAddr:                httpAddr,
		LogLevel:                logLevel,
		ShutdownTimeout:         shutdownTimeout,
		HTTPReadTimeout:         readTimeout,
		HTTPReadHeaderTimeout:   readHeaderTimeout,
		HTTPWriteTimeout:        writeTimeout,
		HTTPIdleTimeout:         idleTimeout,
		DatabaseURL:             databaseURL,
		DatabaseMaxConns:        maxConns,
		DatabaseMinConns:        minConns,
		DatabaseMaxConnLifetime: lifetime,
		DatabaseConnectTimeout:  connectTimeout,
		DatabaseQueryTimeout:    queryTimeout,
		IdempotencyTTL:          ttl,
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

func validateLogLevel(level string) error {
	switch level {
	case "debug", "info", "warn", "error":
		return nil
	default:
		return fmt.Errorf("LOG_LEVEL must be one of: debug, info, warn, error")
	}
}
