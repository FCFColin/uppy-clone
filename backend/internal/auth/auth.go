package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/nicknames"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var tracer = otel.Tracer("github.com/uppy-clone/backend/internal/auth")

type UserDB interface {
	GetUserByID(ctx context.Context, id string) (*domain.User, error)
	CreateUser(ctx context.Context, user *domain.User) error
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	UpdateUserLastLogin(ctx context.Context, id string) error
	AnonymizeUser(ctx context.Context, id string) error
	GetGameResultsByUserID(ctx context.Context, userID string) ([]domain.GameResult, error)
	GetGameSessionsByUserID(ctx context.Context, userID string) ([]domain.GameSession, error)
}

type TokenStore interface {
	StoreMagicToken(ctx context.Context, hashedToken string, data []byte, ttl time.Duration) error
	ConsumeMagicToken(ctx context.Context, tokenHash string) ([]byte, error)
	DeleteMagicToken(ctx context.Context, hashedToken string) error
	CheckRateLimit(ctx context.Context, key string, maxCount int64, window time.Duration) (bool, error)
	IsJWTRevoked(ctx context.Context, jti string) (bool, error)
	RevokeJWT(ctx context.Context, jti string, ttl time.Duration) error
	EnqueueEmail(ctx context.Context, payload []byte) error
}

// TokenVerifier abstracts JWT token verification. *JWTManager satisfies this
// interface; middleware uses it to avoid depending on the concrete type.
type TokenVerifier interface {
	VerifyToken(tokenStr string) (userID, nickname, jti, role string, err error)
}

// JWTRevocationChecker checks if a JWT has been revoked by its jti.
type JWTRevocationChecker interface {
	IsJWTRevoked(ctx context.Context, jti string) (bool, error)
}

const (
	SessionCookie   = "session"
	QuickplayCookie = "quickplay"
)

// tryCookie attempts to read and validate a JWT from the named cookie.
// Extracted to eliminate duplicated cookie-reading code.
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
// A separate AuthenticatedUserFromRequest without revocation was removed (H4)
// because all callers must check revocation.
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
	for _, cookieName := range []string{SessionCookie, QuickplayCookie} {
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

func GetAuthenticatedUser(r *http.Request) (userId, nickname string, ok bool) {
	uid, ok1 := domain.ContextKeyUserID.Value(r.Context())
	nick, ok2 := domain.ContextKeyNickname.Value(r.Context())
	if !ok1 || !ok2 || uid == "" {
		return "", "", false
	}
	return uid, nick, true
}

func GetJTI(r *http.Request) string {
	if jti, ok := domain.ContextKeyJTI.Value(r.Context()); ok {
		return jti
	}
	return ""
}

// WithJTI injects the admin token's jti so that Logout and password-change
// handlers can revoke it.
func WithJTI(ctx context.Context, jti string) context.Context {
	return domain.ContextKeyJTI.WithValue(ctx, jti)
}

func WithAuthenticatedUser(ctx context.Context, userId, nickname string) context.Context {
	ctx = domain.ContextKeyUserID.WithValue(ctx, userId)
	ctx = domain.ContextKeyNickname.WithValue(ctx, nickname)
	return ctx
}

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

type QuickPlayResponse struct {
	UserID   string `json:"userId"`
	Nickname string `json:"nickname"`
	// RefreshToken is set internally for HttpOnly cookie issuance; never serialized.
	RefreshToken string `json:"-"`
}

// QuickPlay creates a temporary user and returns JWT cookie + user info.
// If the request already carries a valid quickplay or session cookie,
// the existing user is returned with a refreshed cookie.
func QuickPlay(db UserDB, jwtMgr *JWTManager, refreshMgr *RefreshTokenManager, revoker JWTRevocationChecker, nickname string, r *http.Request) (*http.Cookie, *QuickPlayResponse, error) {
	ctx := r.Context()

	if uid, nick, ok := AuthenticatedUserFromRequestWithRevocation(r, jwtMgr, revoker); ok {
		user, err := db.GetUserByID(ctx, uid)
		if err != nil {
			slog.Error("quickplay lookup existing user failed", "error", err)
			return nil, nil, fmt.Errorf("lookup existing user: %w", err)
		}
		if user != nil {
			nick = user.Nickname
		}
		return issueQuickPlayCredentials(ctx, jwtMgr, refreshMgr, uid, nick, r)
	}

	nickname = prepareQuickPlayNickname(nickname)
	userId := domain.UUID()

	now := time.Now().Unix()
	user := &domain.User{
		ID:        userId,
		Email:     userId + "@quickplay",
		Nickname:  nickname,
		CreatedAt: now,
	}
	if err := db.CreateUser(ctx, user); err != nil {
		if !errors.Is(err, domain.ErrDuplicateUser) {
			slog.Error("quickplay create user failed", "error", err)
			return nil, nil, fmt.Errorf("create user: %w", err)
		}
	}

	return issueQuickPlayCredentials(ctx, jwtMgr, refreshMgr, userId, nickname, r)
}

func prepareQuickPlayNickname(nickname string) string {
	nickname = sanitizePlayerName(nickname)
	if nickname == "" {
		nickname = nicknames.GenerateRandom(nil)
	}
	return nickname
}

func issueQuickPlayCredentials(ctx context.Context, jwtMgr *JWTManager, refreshMgr *RefreshTokenManager, userID, nickname string, r *http.Request) (*http.Cookie, *QuickPlayResponse, error) {
	token, err := jwtMgr.SignToken(userID, nickname)
	if err != nil {
		slog.Error("quickplay sign token failed", "error", err)
		return nil, nil, fmt.Errorf("sign token: %w", err)
	}

	secure := IsSecure(r)
	cookie := BuildAuthCookie(QuickplayCookie, token, config.CookieMaxAge, secure)

	refreshToken, err := refreshMgr.Generate(ctx, userID)
	if err != nil {
		slog.Error("quickplay generate refresh token failed", "error", err)
		return nil, nil, fmt.Errorf("generate refresh token: %w", err)
	}

	// S-07: Return response WITHOUT the access token in the body
	return cookie, &QuickPlayResponse{UserID: userID, Nickname: nickname, RefreshToken: refreshToken}, nil
}

func ParseQuickPlayRequest(r *http.Request) string {
	var body struct {
		Nickname string `json:"nickname"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return ""
	}
	return body.Nickname
}

type VerifyResponse struct {
	UserID   string `json:"userId"`
	Nickname string `json:"nickname"`
	// RefreshToken is set internally for HttpOnly cookie issuance; never serialized.
	RefreshToken string `json:"-"`
}

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

const (
	gdprFieldEmail     = "email"
	gdprFieldNickname  = "nickname"
	gdprFieldPalette   = "palette"
	gdprFieldCreatedAt = "created_at"
	gdprFieldLastLogin = "last_login"
)

func ExportUserData(ctx context.Context, dataStore UserDB, userID string) (map[string]interface{}, error) {
	user, err := dataStore.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if user == nil {
		return nil, domain.ErrNotFound
	}

	exportData := map[string]interface{}{
		"user": map[string]interface{}{
			"id":               user.ID,
			gdprFieldEmail:     user.Email,
			gdprFieldNickname:  user.Nickname,
			gdprFieldPalette:   user.Palette,
			gdprFieldCreatedAt: user.CreatedAt,
			gdprFieldLastLogin: user.LastLogin,
		},
	}
	// auth-013: Return game results error to caller instead of silently warning.
	// GDPR compliance requires complete data export — silently omitting data
	// is a compliance risk.
	results, err := dataStore.GetGameResultsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("fetch game results for GDPR export: %w", err)
	}
	exportData["game_results"] = results
	// auth-012: Include game sessions in GDPR export for complete data portability.
	sessions, err := dataStore.GetGameSessionsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("fetch game sessions for GDPR export: %w", err)
	}
	exportData["game_sessions"] = sessions
	return exportData, nil
}

func DeleteUserData(ctx context.Context, jwtMgr *JWTManager, refreshMgr *RefreshTokenManager, tokens TokenStore, dataStore UserDB, userID string, r *http.Request) error {
	if refreshMgr != nil {
		_ = refreshMgr.RevokeAllForUser(ctx, userID)
	}
	if revokeErr := RevokeAllTokens(ctx, jwtMgr, refreshMgr, tokens, r); revokeErr != nil {
		slog.Error("failed to revoke tokens during GDPR delete", "user_id", userID, "error", revokeErr)
	}
	if dataStore != nil {
		if err := dataStore.AnonymizeUser(ctx, userID); err != nil {
			return fmt.Errorf("anonymize user: %w", err)
		}
	}
	audit.Log(ctx, audit.AuditEntry{
		Action:    "gdpr_anonymize",
		ActorType: audit.ActorTypeUser,
		ActorID:   userID,
		Resource:  "user/" + userID,
	})
	return nil
}

type RefreshSessionResult struct {
	AccessToken  string
	RefreshToken string
}

// RefreshSession validates and rotates refresh tokens atomically,
// detecting token reuse (theft) and revoking all tokens for the compromised user.
func RefreshSession(ctx context.Context, refreshMgr *RefreshTokenManager, jwtMgr *JWTManager, dataStore UserDB, oldToken string) (*RefreshSessionResult, error) {
	result, err := refreshMgr.ConsumeRefreshToken(ctx, oldToken)
	if err != nil {
		return nil, fmt.Errorf("consume refresh token: %w", err)
	}

	if result.Reused {
		slog.Warn("refresh token reuse detected — revoking all tokens for user",
			"user_id", result.UserID)
		if revokeErr := refreshMgr.RevokeAllForUser(ctx, result.UserID); revokeErr != nil {
			slog.Error("failed to revoke all tokens after reuse detection",
				"user_id", result.UserID, "error", revokeErr)
		}
		return nil, fmt.Errorf("refresh token has already been used")
	}

	user, err := dataStore.GetUserByID(ctx, result.UserID)
	if err != nil {
		return nil, fmt.Errorf("lookup user: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	_ = refreshMgr.RemoveFromUserSet(ctx, result.UserID, oldToken)

	accessToken, err := jwtMgr.SignToken(result.UserID, user.Nickname)
	if err != nil {
		return nil, fmt.Errorf("sign token: %w", err)
	}
	newRefresh, err := refreshMgr.Generate(ctx, result.UserID)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	return &RefreshSessionResult{AccessToken: accessToken, RefreshToken: newRefresh}, nil
}
