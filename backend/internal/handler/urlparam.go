package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// URLParam extracts a named path parameter from the request.
//
// It prefers the Go 1.22+ standard library path value (r.PathValue) so that
// tests can set parameters via r.SetPathValue without a router, and falls back
// to chi.URLParam for production routes served by the chi router. This keeps
// the handler package's tests decoupled from chi-specific context plumbing.
func URLParam(r *http.Request, name string) string {
	if v := r.PathValue(name); v != "" {
		return v
	}
	return chi.URLParam(r, name)
}
