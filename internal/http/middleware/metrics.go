package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

func Metrics(counter upDownCounter, router chi.Router) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route := matchRoute(router, r)
			attrs := metric.WithAttributes(
				attribute.String("http.request.method", r.Method),
				attribute.String("http.route", route),
			)
			if counter != nil {
				counter.Add(r.Context(), 1, attrs)
			}
			defer func() {
				if counter != nil {
					counter.Add(r.Context(), -1, attrs)
				}
				if recovered := recover(); recovered != nil {
					panic(recovered)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
