package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestURLParam_PathValuePriority(t *testing.T) {
	t.Parallel()
	// Go 1.22+ r.PathValue should be preferred over chi.URLParam.
	// We set both: stdlib via SetPathValue, chi via RouteCtxKey context.
	// stdlib value must win.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetPathValue("id", "from-stdlib")

	// Also set chi context to ensure PathValue wins.
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "from-chi")
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	req = req.WithContext(ctx)

	got := URLParam(req, "id")
	if got != "from-stdlib" {
		t.Fatalf("URLParam = %q, want from-stdlib (stdlib path value takes priority)", got)
	}
}

func TestURLParam_FallbackToChi(t *testing.T) {
	t.Parallel()
	// When stdlib PathValue is empty, fall back to chi.URLParam.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "from-chi")
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	req = req.WithContext(ctx)

	got := URLParam(req, "id")
	if got != "from-chi" {
		t.Fatalf("URLParam = %q, want from-chi (chi fallback)", got)
	}
}

func TestURLParam_EmptyWhenNotSet(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	got := URLParam(req, "id")
	if got != "" {
		t.Fatalf("URLParam = %q, want empty", got)
	}
}

func TestURLParam_ChiRouter(t *testing.T) {
	t.Parallel()
	// Integration: chi router sets URLParam in context.
	r := chi.NewRouter()
	r.Get("/users/{id}", func(w http.ResponseWriter, req *http.Request) {
		got := URLParam(req, "id")
		if got != "123" {
			t.Errorf("URLParam = %q, want 123", got)
		}
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users/123", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestURLParam_MultipleParams(t *testing.T) {
	t.Parallel()
	r := chi.NewRouter()
	r.Get("/users/{userId}/posts/{postId}", func(w http.ResponseWriter, req *http.Request) {
		uid := URLParam(req, "userId")
		pid := URLParam(req, "postId")
		if uid != "u1" {
			t.Errorf("userId = %q, want u1", uid)
		}
		if pid != "p2" {
			t.Errorf("postId = %q, want p2", pid)
		}
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users/u1/posts/p2", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestURLParam_StdlibPathValueOnly(t *testing.T) {
	t.Parallel()
	// When using only stdlib (no chi), SetPathValue should work.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetPathValue("code", "ABCDE")

	got := URLParam(req, "code")
	if got != "ABCDE" {
		t.Fatalf("URLParam = %q, want ABCDE", got)
	}
}

func TestURLParam_UnknownParamReturnsEmpty(t *testing.T) {
	t.Parallel()
	// When the chi router is used but the requested param name doesn't exist,
	// URLParam returns empty (chi.URLParam returns "" for unknown params).
	r := chi.NewRouter()
	r.Get("/users/{id}", func(w http.ResponseWriter, req *http.Request) {
		// "name" is not in the route pattern.
		got := URLParam(req, "name")
		if got != "" {
			t.Errorf("URLParam(name) = %q, want empty", got)
		}
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users/123", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}
