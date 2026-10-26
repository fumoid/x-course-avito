package api

import (
	"log/slog"
	"net/http"
)

func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered", "panic", rec, "path", r.URL.Path)
				writeProblem(w, r, problemInternalError, "Internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
