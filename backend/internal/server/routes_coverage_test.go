package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestAllRoutesRegistered handler-024: verifies that every route in the OpenAPI
// spec is registered on the chi router. This is a lightweight, non-integration
// test that catches accidental route removals without requiring a database.
func TestAllRoutesRegistered(t *testing.T) {
	r := newTestRouter(t)

	// expectedRoutes maps "METHOD /path" to true. chi.Walk provides the
	// registered routes; any mismatch (missing or extra) fails the test.
	expectedRoutes := map[string]bool{
		"GET /health/live":                  true,
		"GET /health/ready":                 true,
		"GET /health":                       true,
		"GET /health/degraded":              true,
		"GET /metrics":                      true,
		"POST /api/v1/auth/quickplay":       true,
		"GET /api/v1/auth/check":            true,
		"POST /api/v1/auth/refresh":         true,
		"POST /api/v1/auth/logout":          true,
		"GET /api/v1/user/data":             true,
		"DELETE /api/v1/user/data":          true,
		"GET /api/v1/stats/public":          true,
		"GET /api/v1/leaderboard":           true,
		"GET /api/v1/user/stats":            true,
		"POST /api/v1/registry/create":      true,
		"GET /api/v1/registry/check/{code}": true,
		"GET /api/v1/registry/lobbies":      true,
		"POST /api/v1/registry/match":       true,
		"GET /api/v1/lobby/{code}/ws":       true,
		"POST /api/v1/admin/login":          true,
		"POST /api/v1/admin/logout":         true,
		"GET /api/v1/admin/config":          true,
		"PATCH /api/v1/admin/config":        true,
		"PUT /api/v1/admin/config":          true,
	}

	registered := make(map[string]bool)
	_ = chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		registered[method+" "+route] = true
		return nil
	})

	for route := range expectedRoutes {
		if !registered[route] {
			t.Errorf("handler-024: expected route %q is NOT registered", route)
		}
	}
	for route := range registered {
		if !expectedRoutes[route] {
			// Skip the catch-all static route "GET /*" when FrontendDir is empty
			if route == "GET /*" {
				continue
			}
			// /metrics is registered via r.Handle (all HTTP methods).
			// chi.Walk reports every method; only GET is in the expected set.
			if strings.HasSuffix(route, " /metrics") && !strings.HasPrefix(route, "GET ") {
				continue
			}
			t.Errorf("handler-024: unexpected route %q is registered (not in expected list)", route)
		}
	}
}
