package middleware

import (
	"fmt"
	"net/http"

	"github.com/uppy-clone/backend/internal/domain"
)

// JWTManager defines JWT verification operations needed by middleware.
type JWTManager interface {
	VerifyToken(tokenStr string) (userID, nickname, jti string, err error)
}

// getAuthenticatedUser reads authenticated user info from request context.
func getAuthenticatedUser(r *http.Request) (userID string, nickname string, ok bool) {
	uid, ok1 := domain.ContextKeyUserID.Value(r.Context())
	nick, ok2 := domain.ContextKeyNickname.Value(r.Context())
	if !ok1 || !ok2 || uid == "" {
		return "", "", false
	}
	return uid, nick, true
}

// parseAuthCookie reads the named cookie and verifies it with the given JWT manager.
func parseAuthCookie(r *http.Request, cookieName string, jwtMgr JWTManager) (userID, nickname, jti string, err error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return "", "", "", fmt.Errorf("cookie %s not found: %w", cookieName, err)
	}
	return jwtMgr.VerifyToken(cookie.Value)
}
