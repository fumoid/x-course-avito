package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	api "github.com/fumoid/x-course-avito/internal/generated"
)

func TestRecoverWritesProblem(t *testing.T) {
	t.Parallel()
	inner := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips", nil)
	rec := httptest.NewRecorder()
	Recover(nil)(inner).ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("content-type = %s", rec.Header().Get("Content-Type"))
	}
	var body api.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "internal_error" {
		t.Fatalf("code = %s", body.Code)
	}
}
