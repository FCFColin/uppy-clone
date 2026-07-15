package auth

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/uppy-clone/backend/internal/config"
)

// RevokeAllTokens revokes all tokens (refresh + access) for the current user.
// Extracted to eliminate duplication between Logout and DeleteUserData handlers.
// Returns the error from RevokeAllForUser if one occurs.
func RevokeAllTokens(ctx context.Context, jwtMgr *JWTManager, refreshMgr *RefreshTokenManager, tokens TokenStore, r *http.Request) error {
	if r == nil {
		return nil
	}
	var userID string

	for _, cookieName := range []string{"session", "quickplay"} {
		if cookie, err := r.Cookie(cookieName); err == nil && cookie.Value != "" {
			uid, _, jti, _, err := jwtMgr.VerifyToken(cookie.Value)
			if err == nil {
				if uid != "" {
					userID = uid
				}
				if jti != "" && tokens != nil {
					// auth-019: Use the full AccessTokenTTL as an upper bound for jti revocation.
					// The token may have been issued less than AccessTokenTTL ago, so this keeps
					// the jti in Redis slightly longer than strictly necessary. This is safe —
					// the extra TTL just means the revocation entry lingers harmlessly after
					// the token would have expired anyway. Calculating the exact remaining TTL
					// would require parsing the exp claim from VerifyToken, which currently
					// doesn't expose it.
					_ = tokens.RevokeJWT(ctx, jti, config.AccessTokenTTL)
				}
			}
		}
	}

	// If no userID was found from JWT (e.g. expired access token), try the refresh cookie.
	if userID == "" && refreshMgr != nil {
		refreshToken := RefreshTokenFromRequest(r)
		if refreshToken != "" {
			result, err := refreshMgr.ConsumeRefreshToken(ctx, refreshToken)
			if err == nil && result.UserID != "" {
				userID = result.UserID
			}
		}
	}

	if refreshMgr != nil && userID != "" {
		if err := refreshMgr.RevokeAllForUser(ctx, userID); err != nil {
			slog.Error("failed to revoke all refresh tokens", "user_id", userID, "error", err)
			return err
		}
	}
	return nil
}
