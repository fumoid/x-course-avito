package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

func Logging(log logger, router chi.Router) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := &statusWriter{ResponseWriter: w}
			start := time.Now()
			defer func() {
				status := http.StatusInternalServerError
				recovered := recover()
				if recovered == nil {
					status = sw.code()
				}
				route := routePattern(router, r)
				durationMS := time.Since(start).Milliseconds()
				args := []any{
					"method", r.Method,
					"route", route,
					"status", status,
					"duration_ms", durationMS,
				}
				if log != nil {
					if status >= http.StatusInternalServerError {
						log.ErrorContext(r.Context(), "request handled", args...)
					} else {
						log.InfoContext(r.Context(), "request handled", args...)
					}
				}
				if recovered != nil {
					panic(recovered)
				}
			}()
			next.ServeHTTP(sw, r)
		})
	}
}
