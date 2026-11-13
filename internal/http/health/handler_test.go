package health_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/fumoid/x-course-avito/internal/http/health"
)

func TestHealth(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	pinger := NewMockpinger(ctrl)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	health.New(pinger, time.Second).Health(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestReadyOK(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	pinger := NewMockpinger(ctrl)
	pinger.EXPECT().Ping(gomock.Any()).Return(nil)
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	health.New(pinger, time.Second).Ready(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestReadyUnavailable(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	pinger := NewMockpinger(ctrl)
	pinger.EXPECT().Ping(gomock.Any()).Return(errors.New("db down"))
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	health.New(pinger, time.Second).Ready(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
}
