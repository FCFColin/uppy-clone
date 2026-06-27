package auth

import (
	"net/http"

	"github.com/uppy-clone/backend/internal/requestctx"
)

// IsSecure reports whether the request was made over HTTPS.
// X-Forwarded-Proto is honored only when the peer is a trusted reverse proxy.
func IsSecure(r *http.Request) bool {
	if requestctx.IsTrustedProxy(r.Context()) {
		if proto := r.Header.Get("X-Forwarded-Proto"); proto == "https" {
			return true
		}
	}
	return r.TLS != nil
}
