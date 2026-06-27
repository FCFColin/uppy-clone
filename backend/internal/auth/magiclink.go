package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/idgen"
	"github.com/uppy-clone/backend/internal/requestctx"
	"github.com/uppy-clone/backend/internal/store"
)

// 哨兵错误 — 企业为何需要：字符串比较错误消息容易因拼写/格式差异失效，
// 哨兵错误用 errors.Is 提供稳定的判等语义，便于调用方精确分支处理。
var (
	ErrTooManyRequests = errors.New("too many requests, try again later")
	ErrInvalidEmail    = errors.New("invalid email format")
)

// getOrigin constructs the origin URL from the request, respecting reverse proxy headers.
// Enterprise rationale: Behind reverse proxies (Cloud Run, nginx, Cloudflare),
// r.Host is the internal hostname, not the public URL. X-Forwarded-Host
// contains the original Host header sent by the client. Without this fix,
// magic link URLs point to internal hostnames that are unreachable from
// the user's browser.
func getOrigin(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil && (!requestctx.IsTrustedProxy(r.Context()) || r.Header.Get("X-Forwarded-Proto") == "") {
		scheme = "http"
	}
	if requestctx.IsTrustedProxy(r.Context()) {
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		}
	}
	host := r.Host
	if requestctx.IsTrustedProxy(r.Context()) {
		if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
			host = fwdHost
		}
	}
	return scheme + "://" + host
}

// VerifyResponse is returned after a successful magic-link verification.
type VerifyResponse struct {
	UserID       string `json:"userId"`
	Nickname     string `json:"nickname"`
	RefreshToken string `json:"refreshToken,omitempty"`
}

// magicTokenData is stored in Redis for each magic-link token.
type magicTokenData struct {
	Email     string `json:"email"`
	CreatedAt int64  `json:"createdAt"`
}

// MagicLinkService handles magic-link authentication.
// Circuit breaker protection for the Resend email API is handled by EmailWorker (T22).
type MagicLinkService struct {
}

// NewMagicLinkService creates a MagicLinkService.
func NewMagicLinkService() *MagicLinkService {
	return &MagicLinkService{}
}

// RequestMagicLink sends a magic link email to the user.
// Flow: validate email → rate limit → generate token → hash → store in Redis → send email.
func (s *MagicLinkService) RequestMagicLink(redis *store.RedisStore, db *store.PostgresStore, resendAPIKey, emailFrom, email string, r *http.Request, timeouts config.TimeoutConfig) error {
	if !isValidEmail(email) {
		return ErrInvalidEmail
	}

	ctx := r.Context()
	if err := checkMagicLinkRateLimit(ctx, redis, email); err != nil {
		return err
	}

	token, hashedToken, err := generateMagicLinkToken()
	if err != nil {
		return err
	}

	if err := storeMagicLinkToken(ctx, redis, hashedToken, email); err != nil {
		return err
	}

	if err := enqueueMagicLinkEmail(ctx, redis, r, email, token, hashedToken); err != nil {
		return err
	}

	return nil
}

func checkMagicLinkRateLimit(ctx context.Context, redis *store.RedisStore, email string) error {
	allowed, err := redis.CheckRateLimit(ctx, "ml:"+email, 5, config.MagicLinkTTL)
	if err != nil {
		return fmt.Errorf("rate limit check: %w", err)
	}
	if !allowed {
		return ErrTooManyRequests
	}
	return nil
}

func generateMagicLinkToken() (token, hashedToken string, err error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}
	token = hex.EncodeToString(tokenBytes)
	return token, HashToken(token), nil
}

func storeMagicLinkToken(ctx context.Context, redis *store.RedisStore, hashedToken, email string) error {
	encryptedEmail, encErr := crypto.Encrypt(email)
	if encErr != nil {
		return fmt.Errorf("encrypt email: %w", encErr)
	}
	data := magicTokenData{
		Email:     encryptedEmail,
		CreatedAt: time.Now().UnixMilli(),
	}
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal token data: %w", err)
	}
	if err := redis.StoreMagicToken(ctx, hashedToken, dataBytes, config.MagicLinkTTL); err != nil {
		return fmt.Errorf("store magic token: %w", err)
	}
	return nil
}

func enqueueMagicLinkEmail(ctx context.Context, redis *store.RedisStore, r *http.Request, email, token, hashedToken string) error {
	origin := getOrigin(r)
	magicLinkURL := origin + "/api/v1/auth/verify?token=" + token

	emailPayload := map[string]interface{}{
		"to":      email,
		"subject": "Your Login Link",
		"body":    fmt.Sprintf(`<p>Click <a href='%s'>here</a> to log in. Expires in 15 minutes.</p>`, magicLinkURL),
	}
	payloadJSON, err := json.Marshal(emailPayload)
	if err != nil {
		return fmt.Errorf("marshal email payload: %w", err)
	}

	// P4-6.2: Saga 补偿模式 — 邮件入队失败时删除已存储的 Redis token，
	// 避免用户收到无法验证的魔法链接（token 已存但邮件未发送）。
	if err := redis.EnqueueEmail(ctx, payloadJSON); err != nil {
		_ = redis.DeleteMagicToken(ctx, hashedToken)
		return fmt.Errorf("enqueue email: %w", err)
	}
	return nil
}

// VerifyMagicLink verifies a magic link token and creates/updates user.
// Flow: hash token → lookup Redis → parse data → delete token → find/create user → sign JWT → set cookie.
func VerifyMagicLink(redis *store.RedisStore, db *store.PostgresStore, jwtMgr *JWTManager, refreshMgr *RefreshTokenManager, token string) (*http.Cookie, *VerifyResponse, error) {
	ctx := context.Background()

	email, err := validateMagicToken(ctx, redis, token)
	if err != nil {
		return nil, nil, err
	}

	user, err := findOrCreateUserByEmail(ctx, db, email)
	if err != nil {
		return nil, nil, err
	}

	// Update last_login
	if err := db.UpdateUserLastLogin(ctx, user.ID); err != nil {
		// Non-fatal — log but continue
		_ = err
	}

	// 6. Sign JWT, set HttpOnly session cookie
	jwtToken, err := jwtMgr.SignToken(user.ID, user.Nickname)
	if err != nil {
		return nil, nil, fmt.Errorf("sign token: %w", err)
	}

	cookie := BuildAuthCookie("session", jwtToken, config.CookieMaxAge, true) // 15min matches access token TTL

	// Generate refresh token
	refreshToken, err := refreshMgr.Generate(ctx, user.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("generate refresh token: %w", err)
	}

	// 7. Return user info
	return cookie, &VerifyResponse{UserID: user.ID, Nickname: user.Nickname, RefreshToken: refreshToken}, nil
}

// validateMagicToken hashes the token, looks it up in Redis, validates expiry,
// decrypts the stored email, and deletes the one-time-use token.
func validateMagicToken(ctx context.Context, redis *store.RedisStore, token string) (string, error) {
	hashedToken := HashToken(token)

	dataBytes, err := redis.GetMagicToken(ctx, hashedToken)
	if err != nil {
		return "", fmt.Errorf("lookup token: %w", err)
	}
	if dataBytes == nil {
		return "", fmt.Errorf("invalid or expired token")
	}

	var data magicTokenData
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		_ = redis.DeleteMagicToken(ctx, hashedToken)
		return "", fmt.Errorf("invalid token data")
	}

	// Decrypt email from Redis
	// 企业为何需要：email 是 PII，Redis 数据库泄露即暴露用户邮箱。字段级加密提供纵深防御。
	decryptedEmail, decErr := crypto.Decrypt(data.Email)
	if decErr != nil {
		// If decryption fails, the value may be plaintext (legacy data)
		decryptedEmail = data.Email
	}
	data.Email = decryptedEmail

	// Verify not expired (15 minutes)
	if data.CreatedAt+int64(config.MagicLinkTTL/time.Millisecond) < time.Now().UnixMilli() {
		_ = redis.DeleteMagicToken(ctx, hashedToken)
		return "", fmt.Errorf("invalid or expired token")
	}

	// Delete token from Redis (one-time use)
	if err := redis.DeleteMagicToken(ctx, hashedToken); err != nil {
		return "", fmt.Errorf("delete token: %w", err)
	}

	return data.Email, nil
}

// findOrCreateUserByEmail looks up a user by email, creating a new one if not found.
func findOrCreateUserByEmail(ctx context.Context, db *store.PostgresStore, email string) (*domain.User, error) {
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
			ID:        idgen.UUID(),
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
