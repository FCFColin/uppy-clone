package auth

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/store"
)

// RevokeAllTokens revokes all tokens (refresh + access) for the current user.
// Extracted to eliminate duplication between Logout and DeleteUserData handlers.
func RevokeAllTokens(ctx context.Context, jwtMgr *JWTManager, refreshMgr *RefreshTokenManager, redis *store.RedisStore, r *http.Request) {
	var userID string

	for _, cookieName := range []string{"session", "quickplay"} {
		if cookie, err := r.Cookie(cookieName); err == nil && cookie.Value != "" {
			uid, _, jti, err := jwtMgr.VerifyToken(cookie.Value)
			if err == nil {
				if uid != "" {
					userID = uid
				}
				if jti != "" && redis != nil {
					_ = redis.RevokeJWT(ctx, jti, config.AccessTokenTTL)
				}
			}
		}
	}

	if refreshMgr != nil && userID != "" {
		if err := refreshMgr.RevokeAllForUser(ctx, userID); err != nil {
			slog.Warn("failed to revoke all refresh tokens", "user_id", userID, "error", err)
		}
	}
}
