package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://tripgo:tripgo@localhost:5432/tripgo?sslmode=disable")
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("SHUTDOWN_TIMEOUT", "")
	t.Setenv("HTTP_READ_TIMEOUT", "")
	t.Setenv("HTTP_READ_HEADER_TIMEOUT", "")
	t.Setenv("HTTP_WRITE_TIMEOUT", "")
	t.Setenv("HTTP_IDLE_TIMEOUT", "")
	t.Setenv("DATABASE_MAX_CONNS", "")
	t.Setenv("DATABASE_MIN_CONNS", "")
	t.Setenv("DATABASE_MAX_CONN_LIFETIME", "")
	t.Setenv("DATABASE_CONNECT_TIMEOUT", "")
	t.Setenv("DATABASE_QUERY_TIMEOUT", "")
	t.Setenv("IDEMPOTENCY_TTL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTP_ADDR = %s", cfg.HTTPAddr)
	}
	if cfg.HTTPReadTimeout != 10*time.Second {
		t.Fatalf("HTTP_READ_TIMEOUT = %s", cfg.HTTPReadTimeout)
	}
	if cfg.IdempotencyTTL != 24*time.Hour {
		t.Fatalf("IDEMPOTENCY_TTL = %s", cfg.IdempotencyTTL)
	}
}

func TestLoadRejectsZeroTimeout(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://tripgo:tripgo@localhost:5432/tripgo?sslmode=disable")
	t.Setenv("HTTP_READ_TIMEOUT", "0s")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for zero timeout")
	}
}

func TestLoadRejectsMinGreaterThanMax(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://tripgo:tripgo@localhost:5432/tripgo?sslmode=disable")
	t.Setenv("DATABASE_MAX_CONNS", "2")
	t.Setenv("DATABASE_MIN_CONNS", "5")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for min > max")
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing DATABASE_URL")
	}
}
