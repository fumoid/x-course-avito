package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func matchRoute(router chi.Router, r *http.Request) string {
	if router == nil {
		return unknownRoute
	}
	rctx := chi.NewRouteContext()
	if router.Match(rctx, r.Method, r.URL.Path) {
		if pattern := rctx.RoutePattern(); pattern != "" {
			return pattern
		}
	}
	return unknownRoute
}

func routePattern(router chi.Router, r *http.Request) string {
	if rc := chi.RouteContext(r.Context()); rc != nil {
		if pattern := rc.RoutePattern(); pattern != "" {
			return pattern
		}
	}
	return matchRoute(router, r)
}
