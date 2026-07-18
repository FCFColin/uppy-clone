package auth

import (
	"net/http"
	"strings"

	"github.com/uppy-clone/backend/internal/domain"
)

// IsSecure reports whether the request was made over HTTPS.
// X-Forwarded-Proto is honored only when the peer is a trusted reverse proxy.
func IsSecure(r *http.Request) bool {
	if domain.IsTrustedProxy(r.Context()) {
		if proto := r.Header.Get("X-Forwarded-Proto"); strings.EqualFold(proto, "https") {
			return true
		}
	}
	return r.TLS != nil
}
