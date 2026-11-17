package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	pushPath    = "/api/v1/push"
	kindRequest = "REQUEST_POSITION"
	spanPushReq = "push.request"
)

type logger interface {
	WarnContext(ctx context.Context, msg string, args ...any)
	ErrorContext(ctx context.Context, msg string, args ...any)
}

type Client struct {
	http    *http.Client
	baseURL string
	log     logger
	tracer  trace.Tracer
}

type pushRequest struct {
	RequestID   string            `json:"request_id"`
	RecipientID string            `json:"recipient_id"`
	Kind        string            `json:"kind"`
	Data        map[string]string `json:"data"`
}

func New(baseURL string, timeout time.Duration, log logger, tracer trace.Tracer) *Client {
	return &Client{
		http:    &http.Client{Timeout: timeout},
		baseURL: strings.TrimRight(baseURL, "/"),
		log:     log,
		tracer:  tracer,
	}
}

func (c *Client) RequestPosition(ctx context.Context, tripID, driverID uuid.UUID) error {
	ctx, span := c.tracer.Start(ctx, spanPushReq, trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	body, err := json.Marshal(pushRequest{
		RequestID:   uuid.NewString(),
		RecipientID: driverID.String(),
		Kind:        kindRequest,
		Data:        map[string]string{"trip_id": tripID.String()},
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "marshal")
		return fmt.Errorf("marshal push request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+pushPath, bytes.NewReader(body))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "build request")
		return fmt.Errorf("build push request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := c.http.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "transport")
		c.log.WarnContext(ctx, "push service unavailable", "error", err, "trip_id", tripID.String())
		return fmt.Errorf("push service transport: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusAccepted {
		return nil
	}
	err = StatusError{Status: resp.StatusCode}
	span.RecordError(err)
	span.SetStatus(codes.Error, resp.Status)
	if resp.StatusCode >= 500 {
		c.log.WarnContext(ctx, "push service server error", "status", resp.StatusCode, "trip_id", tripID.String())
	} else {
		c.log.ErrorContext(ctx, "push service client error", "status", resp.StatusCode, "trip_id", tripID.String())
	}
	return err
}

func (c *Client) CloseIdleConnections() {
	if c.http != nil {
		c.http.CloseIdleConnections()
	}
}

type StatusError struct {
	Status int
}

func (e StatusError) Error() string {
	switch {
	case e.Status >= 500:
		return fmt.Sprintf("push service server error: %d", e.Status)
	case e.Status >= 400:
		return fmt.Sprintf("push service client error: %d", e.Status)
	default:
		return fmt.Sprintf("push service unexpected status: %d", e.Status)
	}
}
