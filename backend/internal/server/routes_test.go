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

func TestSetupStaticRoutes_AbsStaticDirError(t *testing.T) {
	dir := t.TempDir()
	prev := filepathAbsFn
	calls := 0
	filepathAbsFn = func(path string) (string, error) {
		calls++
		if calls == 2 {
			return "", errors.New("abs static dir failed")
		}
		return filepath.Abs(path)
	}
	t.Cleanup(func() { filepathAbsFn = prev })

	r := chi.NewRouter()
	setupStaticRoutes(r, &handler.Config{FrontendDir: dir})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/page.html", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 on static dir abs error", rec.Code)
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

func TestSetupStaticRoutes_ExtensionlessFileNoCache(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "about"), []byte("about page"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	setupStaticRoutes(r, &handler.Config{FrontendDir: dir})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/about", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache for extensionless file", cc)
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
