package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/nicknames"
)

// QuickPlayResponse is returned after a successful quick-play registration.
type QuickPlayResponse struct {
	UserID   string `json:"userId"`
	Nickname string `json:"nickname"`
	// RefreshToken is set internally for HttpOnly cookie issuance; never serialized.
	RefreshToken string `json:"-"`
}

// QuickPlay creates a temporary user and returns JWT cookie + user info.
// If the request already carries a valid quickplay or session cookie,
// the existing user is returned with a refreshed cookie (matching TS behavior).
func QuickPlay(db UserDB, jwtMgr *JWTManager, refreshMgr *RefreshTokenManager, revoker JWTRevocationChecker, nickname string, r *http.Request) (*http.Cookie, *QuickPlayResponse, error) {
	ctx := r.Context()

	// Check if already authenticated — reuse existing user (cookie or middleware context)
	if uid, nick, ok := AuthenticatedUserFromRequestWithRevocation(r, jwtMgr, revoker); ok {
		user, err := db.GetUserByID(ctx, uid)
		if err != nil {
			return nil, nil, fmt.Errorf("lookup existing user: %w", err)
		}
		if user != nil {
			nick = user.Nickname
		}
		return issueQuickPlayCredentials(ctx, jwtMgr, refreshMgr, uid, nick, r)
	}

	// Sanitize nickname
	nickname = prepareQuickPlayNickname(nickname)

	// Generate UUID for new user
	userId := domain.UUID()

	// Create temporary user (email = userId@quickplay)
	now := time.Now().Unix()
	user := &domain.User{
		ID:        userId,
		Email:     userId + "@quickplay",
		Nickname:  nickname,
		CreatedAt: now,
	}
	if err := db.CreateUser(ctx, user); err != nil {
		// INSERT OR IGNORE equivalent — try to continue on duplicate
		if !errors.Is(err, domain.ErrDuplicateUser) {
			return nil, nil, fmt.Errorf("create user: %w", err)
		}
	}

	return issueQuickPlayCredentials(ctx, jwtMgr, refreshMgr, userId, nickname, r)
}

// prepareQuickPlayNickname sanitizes the nickname, truncates to MaxNicknameLen,
// and generates a random name if empty.
func prepareQuickPlayNickname(nickname string) string {
	nickname = sanitizePlayerName(nickname)
	if nickname == "" {
		nickname = nicknames.GenerateRandom(nil)
	}
	return nickname
}

// issueQuickPlayCredentials signs the JWT, builds the quickplay cookie, generates
// a refresh token, and returns the cookie + response.
func issueQuickPlayCredentials(ctx context.Context, jwtMgr *JWTManager, refreshMgr *RefreshTokenManager, userID, nickname string, r *http.Request) (*http.Cookie, *QuickPlayResponse, error) {
	token, err := jwtMgr.SignToken(userID, nickname)
	if err != nil {
		return nil, nil, fmt.Errorf("sign token: %w", err)
	}

	secure := IsSecure(r)
	cookie := BuildAuthCookie(quickplayCookie, token, config.CookieMaxAge, secure) // 15min matches access token TTL

	refreshToken, err := refreshMgr.Generate(ctx, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("generate refresh token: %w", err)
	}

	// S-07: Return response WITHOUT the access token in the body
	return cookie, &QuickPlayResponse{UserID: userID, Nickname: nickname, RefreshToken: refreshToken}, nil
}

// ParseQuickPlayRequest extracts the optional nickname from the request body.
func ParseQuickPlayRequest(r *http.Request) string {
	var body struct {
		Nickname string `json:"nickname"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return ""
	}
	return body.Nickname
}
