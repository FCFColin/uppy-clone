package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
)

// VerifyResponse is returned after a successful session issuance.
type VerifyResponse struct {
	UserID   string `json:"userId"`
	Nickname string `json:"nickname"`
	// RefreshToken is set internally for HttpOnly cookie issuance; never serialized.
	RefreshToken string `json:"-"`
}

// findOrCreateUserByEmail looks up a user by email, creating a new one if not found.
func findOrCreateUserByEmail(ctx context.Context, db UserDB, email string) (*domain.User, error) {
	user, err := db.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("lookup user: %w", err)
	}

	if user == nil {
		nickname := email
		if atIdx := strings.Index(nickname, "@"); atIdx > 0 {
			nickname = nickname[:atIdx]
		}
		now := time.Now().Unix()
		user = &domain.User{
			ID:        domain.UUID(),
			Email:     email,
			Nickname:  nickname,
			CreatedAt: now,
		}
		if err := db.CreateUser(ctx, user); err != nil {
			return nil, fmt.Errorf("create user: %w", err)
		}
	}

	return user, nil
}

// issueMagicLinkSession signs a JWT, generates a refresh token, and builds the
// session cookie. Despite the legacy name, this is the generic session-issuance
// helper.
func issueMagicLinkSession(ctx context.Context, db UserDB, jwtMgr *JWTManager, refreshMgr *RefreshTokenManager, user *domain.User, r *http.Request) (*http.Cookie, *VerifyResponse, error) {
	if err := db.UpdateUserLastLogin(ctx, user.ID); err != nil {
		slog.WarnContext(ctx, "failed to update last login", "error", err, "user_id", user.ID)
	}

	jwtToken, err := jwtMgr.SignToken(user.ID, user.Nickname)
	if err != nil {
		return nil, nil, fmt.Errorf("sign token: %w", err)
	}

	secure := IsSecure(r)
	cookie := BuildAuthCookie("session", jwtToken, config.CookieMaxAge, secure)

	refreshToken, err := refreshMgr.Generate(ctx, user.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("generate refresh token: %w", err)
	}

	return cookie, &VerifyResponse{UserID: user.ID, Nickname: user.Nickname, RefreshToken: refreshToken}, nil
}
