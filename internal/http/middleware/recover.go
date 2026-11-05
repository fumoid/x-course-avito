package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/fumoid/x-course-avito/internal/http/domainerror"
)

func Recover(log logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if log != nil {
					log.ErrorContext(r.Context(), "panic recovered", "panic", rec, "stack", string(debug.Stack()))
				}
				domainerror.WriteInternal(w, r)
			}()
			next.ServeHTTP(w, r)
		})
	}
}
