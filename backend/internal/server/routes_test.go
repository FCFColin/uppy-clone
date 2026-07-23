package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/uppy-clone/backend/internal/handler"
)

func TestSetupStaticRoutes_AbsError(t *testing.T) {
	dir := t.TempDir()
	prev := filepathAbsFn
	filepathAbsFn = func(string) (string, error) {
		return "", errors.New("abs failed")
	}
	t.Cleanup(func() { filepathAbsFn = prev })

	r := chi.NewRouter()
	setupStaticRoutes(r, &handler.Config{FrontendDir: dir})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/page.html", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 on abs error", rec.Code)
	}
}

func TestSetupStaticRoutes_EmptyFrontendDir(t *testing.T) {
	r := chi.NewRouter()
	setupStaticRoutes(r, &handler.Config{FrontendDir: ""})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when FrontendDir empty", rec.Code)
	}
}

func TestSetupStaticRoutes_ServesAssetWithCache(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("console.log('ok')"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	setupStaticRoutes(r, &handler.Config{FrontendDir: dir})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "console.log('ok')" {
		t.Fatalf("body = %q", rec.Body.String())
	}
	if cc := rec.Header().Get("Cache-Control"); cc == "" || cc == "no-cache" {
		t.Fatalf("Cache-Control = %q, want public max-age", cc)
	}
}

func TestSetupStaticRoutes_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	setupStaticRoutes(r, &handler.Config{FrontendDir: dir})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/../../etc/passwd", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 404 or 400 for traversal", rec.Code)
	}
}

func TestSetupStaticRoutes_AbsPathOutsideStaticDir(t *testing.T) {
	dir := t.TempDir()
	prev := filepathAbsFn
	filepathAbsFn = func(path string) (string, error) {
		if strings.Contains(path, "secret") {
			return "/outside/secret/file", nil
		}
		return filepath.Abs(dir)
	}
	t.Cleanup(func() { filepathAbsFn = prev })

	r := chi.NewRouter()
	setupStaticRoutes(r, &handler.Config{FrontendDir: dir})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/secret/file", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for path outside static dir", rec.Code)
	}
}

func TestSetupStaticRoutes_ServesHTMLWithNoCache(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "page.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	setupStaticRoutes(r, &handler.Config{FrontendDir: dir})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/page.html", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache for .html", cc)
	}
}

func TestSetupStaticRoutes_NoIndexReturns404(t *testing.T) {
	dir := t.TempDir()
	r := chi.NewRouter()
	setupStaticRoutes(r, &handler.Config{FrontendDir: dir})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 when index.html missing", rec.Code)
	}
}

func TestSetupStaticRoutes_ServesIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	setupStaticRoutes(r, &handler.Config{FrontendDir: dir})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/unknown-route", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 SPA fallback", rec.Code)
	}
	if rec.Body.String() != "hello" {
		t.Errorf("body = %q", rec.Body.String())
	}
}

func TestSetupStaticRoutes_DirectoryRequest(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("index"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	setupStaticRoutes(r, &handler.Config{FrontendDir: dir})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "index" {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

// TestAllRoutesRegistered verifies that every route in the OpenAPI spec is
// registered on the chi router. This is a lightweight, non-integration test
// that catches accidental route removals without requiring a database.
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
			t.Errorf("expected route %q is NOT registered", route)
		}
	}
	for route := range registered {
		if !expectedRoutes[route] {
			// Skip the catch-all static route "GET /*" when FrontendDir is empty.
			if route == "GET /*" {
				continue
			}
			// /metrics is registered via r.Handle (all HTTP methods).
			// chi.Walk reports every method; only GET is in the expected set.
			if strings.HasSuffix(route, " /metrics") && !strings.HasPrefix(route, "GET ") {
				continue
			}
			t.Errorf("unexpected route %q is registered (not in expected list)", route)
		}
	}
}
