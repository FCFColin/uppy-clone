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
