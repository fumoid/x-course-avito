package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const unknownRoute = "unknown"

func Tracing(router chi.Router) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		otelMiddleware := otelhttp.NewMiddleware("http.server",
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return r.Method + " " + matchRoute(router, r)
			}),
			otelhttp.WithMetricAttributesFn(func(r *http.Request) []attribute.KeyValue {
				return []attribute.KeyValue{
					attribute.String("http.route", matchRoute(router, r)),
					attribute.String("http.request.method", r.Method),
				}
			}),
		)
		return otelMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := &statusWriter{ResponseWriter: w}
			defer func() {
				recovered := recover()
				route := routePattern(router, r)
				span := trace.SpanFromContext(r.Context())
				status := sw.code()
				if recovered != nil {
					status = http.StatusInternalServerError
				}
				if span.IsRecording() {
					span.SetName(r.Method + " " + route)
					span.SetAttributes(
						attribute.String("http.route", route),
						attribute.Int("http.response.status_code", status),
					)
					if status >= http.StatusInternalServerError {
						span.SetStatus(codes.Error, http.StatusText(status))
					}
				}
				if labeler, ok := otelhttp.LabelerFromContext(r.Context()); ok {
					labeler.Add(attribute.String("http.route", route))
				}
				if recovered != nil {
					panic(recovered)
				}
			}()
			next.ServeHTTP(sw, r)
		}))
	}
}
