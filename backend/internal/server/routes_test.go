package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/uppy-clone/backend/internal/handler"
)

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
