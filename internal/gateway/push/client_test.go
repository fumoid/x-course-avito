package push_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/fumoid/x-course-avito/internal/gateway/push"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRequestPositionSuccess(t *testing.T) {
	t.Parallel()
	tripID := uuid.New()
	driverID := uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/push" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("content-type = %s", r.Header.Get("Content-Type"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["kind"] != "REQUEST_POSITION" {
			t.Fatalf("kind = %v", body["kind"])
		}
		if body["recipient_id"] != driverID.String() {
			t.Fatalf("recipient = %v", body["recipient_id"])
		}
		data, _ := body["data"].(map[string]any)
		if data["trip_id"] != tripID.String() {
			t.Fatalf("data = %v", body["data"])
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"message_id":"1","accepted_at":"2026-11-17T00:00:00Z"}`))
	}))
	t.Cleanup(server.Close)

	client := push.New(server.URL, time.Second, testLogger(), noop.NewTracerProvider().Tracer("test"))
	if err := client.RequestPosition(context.Background(), tripID, driverID); err != nil {
		t.Fatal(err)
	}
}

func TestRequestPositionServerError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("nope"))
	}))
	t.Cleanup(server.Close)

	client := push.New(server.URL, time.Second, testLogger(), noop.NewTracerProvider().Tracer("test"))
	err := client.RequestPosition(context.Background(), uuid.New(), uuid.New())
	var status push.StatusError
	if !errors.As(err, &status) || status.Status != http.StatusServiceUnavailable {
		t.Fatalf("got %v", err)
	}
}

func TestRequestPositionClientError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(server.Close)

	client := push.New(server.URL, time.Second, testLogger(), noop.NewTracerProvider().Tracer("test"))
	err := client.RequestPosition(context.Background(), uuid.New(), uuid.New())
	var status push.StatusError
	if !errors.As(err, &status) || status.Status != http.StatusBadRequest {
		t.Fatalf("got %v", err)
	}
}

func TestRequestPositionTransportError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close()

	client := push.New(server.URL, time.Second, testLogger(), noop.NewTracerProvider().Tracer("test"))
	err := client.RequestPosition(context.Background(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected transport error")
	}
	var status push.StatusError
	if errors.As(err, &status) {
		t.Fatalf("status error, want transport: %v", err)
	}
}

func TestRequestPositionClosesBody(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"message_id":"1","accepted_at":"2026-11-17T00:00:00Z"}`))
	}))
	t.Cleanup(server.Close)

	client := push.New(server.URL, time.Second, testLogger(), noop.NewTracerProvider().Tracer("test"))
	if err := client.RequestPosition(context.Background(), uuid.New(), uuid.New()); err != nil {
		t.Fatal(err)
	}
	client.CloseIdleConnections()
}
