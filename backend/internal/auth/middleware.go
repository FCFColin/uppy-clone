package auth

import (
	"context"
	"net/http"

	"github.com/uppy-clone/backend/internal/domain"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var tracer = otel.Tracer("github.com/uppy-clone/backend/internal/auth")

// tryCookie attempts to read and validate a JWT from the named cookie.
// Returns userID, nickname, jti, role if valid, empty strings otherwise.
// Extracted to eliminate duplicated cookie-reading code in authenticatedUserFromCookies.
func tryCookie(r *http.Request, jwtMgr *JWTManager, name string) (userID, nickname, jti, role string) {
	cookie, err := r.Cookie(name)
	if err != nil || cookie.Value == "" {
		return "", "", "", ""
	}
	uid, nick, j, rl, err := jwtMgr.VerifyToken(cookie.Value)
	if err != nil {
		return "", "", "", ""
	}
	return uid, nick, j, rl
}

// AuthenticatedUserFromRequestWithRevocation rejects revoked JWT cookies (logout-safe).
// Note: A separate AuthenticatedUserFromRequest without revocation was removed (H4)
// because all callers must check revocation. Use this function with a revoker instead.
func AuthenticatedUserFromRequestWithRevocation(r *http.Request, jwtMgr *JWTManager, rev JWTRevocationChecker) (userID, nickname string, ok bool) {
	return authenticatedUserFromCookies(r, jwtMgr, rev)
}

func authenticatedUserFromCookies(r *http.Request, jwtMgr *JWTManager, rev JWTRevocationChecker) (userID, nickname string, ok bool) {
	ctx, span := tracer.Start(r.Context(), "auth.authenticated_user_from_cookies")
	defer span.End()
	if uid, nick, ctxOK := GetAuthenticatedUser(r); ctxOK {
		span.SetAttributes(attribute.Bool("from_context", true))
		return uid, nick, true
	}
	if jwtMgr == nil {
		span.SetAttributes(attribute.Bool("jwt_mgr_nil", true))
		return "", "", false
	}
	for _, cookieName := range []string{"session", "quickplay"} {
		uid, nick, jti, _ := tryCookie(r, jwtMgr, cookieName)
		if uid == "" {
			continue
		}
		if rev != nil {
			revoked, revErr := rev.IsJWTRevoked(ctx, jti)
			if revErr != nil || revoked {
				continue
			}
		}
		span.SetAttributes(attribute.String("user_id", uid))
		return uid, nick, true
	}
	return "", "", false
}

// GetAuthenticatedUser extracts user info from request context (set by AuthMiddleware).
// Returns userId, nickname, and ok=true if authenticated.
func GetAuthenticatedUser(r *http.Request) (userId, nickname string, ok bool) {
	uid, ok1 := domain.ContextKeyUserID.Value(r.Context())
	nick, ok2 := domain.ContextKeyNickname.Value(r.Context())
	if !ok1 || !ok2 || uid == "" {
		return "", "", false
	}
	return uid, nick, true
}

// GetJTI extracts the JWT ID from request context (set by AuthMiddleware).
func GetJTI(r *http.Request) string {
	if jti, ok := domain.ContextKeyJTI.Value(r.Context()); ok {
		return jti
	}
	return ""
}

// WithJTI returns a new context with the given jti value set.
// Used by adminAuthMiddleware to inject the admin token's jti so that
// Logout and password-change handlers can revoke it.
func WithJTI(ctx context.Context, jti string) context.Context {
	return domain.ContextKeyJTI.WithValue(ctx, jti)
}

// WithAuthenticatedUser returns a new context with userId and nickname values set.
// This is the inverse of GetAuthenticatedUser and is primarily used in tests.
func WithAuthenticatedUser(ctx context.Context, userId, nickname string) context.Context {
	ctx = domain.ContextKeyUserID.WithValue(ctx, userId)
	ctx = domain.ContextKeyNickname.WithValue(ctx, nickname)
	return ctx
}
