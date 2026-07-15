package middleware

import (
	"fmt"
	"net/http"
)

// JWTManager defines JWT verification operations needed by middleware.
type JWTManager interface {
	VerifyToken(tokenStr string) (userID, nickname, jti, role string, err error)
}

// parseAuthCookie reads the named cookie and verifies it with the given JWT manager.
func parseAuthCookie(r *http.Request, cookieName string, jwtMgr JWTManager) (userID, nickname, jti, role string, err error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return "", "", "", "", fmt.Errorf("cookie %s not found: %w", cookieName, err)
	}
	return jwtMgr.VerifyToken(cookie.Value)
}
