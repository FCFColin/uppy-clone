package middleware

import (
	"fmt"
	"net/http"

	"github.com/uppy-clone/backend/internal/auth"
)

// parseAuthCookie reads the named cookie and verifies it with the given token verifier.
//
// RO-051 (interface segregation): the parameter is auth.TokenVerifier rather
// than *auth.JWTManager so that middleware depends on the narrow capability
// it needs (VerifyToken) and test doubles can be injected without spinning up
// a real JWTManager. *auth.JWTManager already satisfies auth.TokenVerifier.
func parseAuthCookie(r *http.Request, cookieName string, verifier auth.TokenVerifier) (userID, nickname, jti, role string, err error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return "", "", "", "", fmt.Errorf("cookie %s not found: %w", cookieName, err)
	}
	return verifier.VerifyToken(cookie.Value)
}
