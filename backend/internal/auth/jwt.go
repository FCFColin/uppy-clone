package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/validate"
)

// JWTManager handles JWT signing and verification using HMAC-SHA256.
// 支持密钥轮换：primarySecret 用于签发新 token，previousSecret（若设置）仅用于验证旧 token。
type JWTManager struct {
	primarySecret  []byte
	previousSecret []byte // nil if no rotation
}

// NewJWTManager creates a JWTManager with the given secret string.
// 企业为何需要：HS256 要求至少 256 位（32 字节）密钥。短密钥可被暴力破解，导致 JWT 伪造。
func NewJWTManager(secret string) *JWTManager {
	if len(secret) < 32 {
		panic(fmt.Sprintf("JWT_SECRET must be at least 32 bytes (256 bits) for HS256, got %d bytes", len(secret)))
	}
	return &JWTManager{primarySecret: []byte(secret)}
}

// NewJWTManagerWithRotation creates a JWT manager with key rotation support.
// Tokens signed with previousSecret are still validated, but new tokens use primarySecret.
// 企业为何需要：密钥轮换要求旧 token 在过渡期内仍可验证，避免轮换瞬间所有用户被登出。
func NewJWTManagerWithRotation(primarySecret, previousSecret string) *JWTManager {
	m := NewJWTManager(primarySecret)
	if previousSecret != "" {
		m.previousSecret = []byte(previousSecret)
	}
	return m
}

// customClaims extends jwt.RegisteredClaims with nickname.
type customClaims struct {
	Nickname string `json:"nickname"`
	jwt.RegisteredClaims
}

// SignToken creates a JWT with userId (sub), nickname, and jti claims.
// Access token expires in 15 minutes; use refresh tokens for longer sessions.
// 企业为何需要：无撤销机制的 JWT 意味着被盗 token 在过期前持续有效。JWT 撤销列表是登出安全的行业标准实现，
// 用 Redis SET + TTL 实现最小性能开销。jti (JWT ID) 是 RFC 7519 标准字段，用于唯一标识每个 token。
func (m *JWTManager) SignToken(userId, nickname string) (string, error) {
	now := time.Now()
	jti, err := generateJTI()
	if err != nil {
		return "", fmt.Errorf("generate jti: %w", err)
	}
	claims := customClaims{
		Nickname: nickname,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   userId,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(config.AccessTokenTTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.primarySecret)
}

// Secret returns the JWT signing secret (for admin token operations).
func (m *JWTManager) Secret() []byte {
	return m.primarySecret
}

// BuildAuthCookie creates an HttpOnly, SameSite=Lax, Secure cookie
// matching the TypeScript buildAuthCookie behavior.
func BuildAuthCookie(name, value string, maxAge int, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}

// RefreshCookieName is the HttpOnly cookie storing the long-lived refresh token.
const RefreshCookieName = "refresh"

// BuildRefreshCookie creates the refresh-token HttpOnly cookie.
func BuildRefreshCookie(value string, secure bool) *http.Cookie {
	maxAge := int(config.RefreshTokenTTL.Seconds())
	if value == "" {
		maxAge = -1
	}
	return BuildAuthCookie(RefreshCookieName, value, maxAge, secure)
}

// RefreshTokenFromRequest reads the refresh token from the HttpOnly cookie.
func RefreshTokenFromRequest(r *http.Request) string {
	cookie, err := r.Cookie(RefreshCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// ParseAuthCookie extracts and verifies JWT from the named cookie.
func ParseAuthCookie(r *http.Request, cookieName string, jwtManager *JWTManager) (userId, nickname, jti string, err error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return "", "", "", fmt.Errorf("cookie %s not found: %w", cookieName, err)
	}
	return jwtManager.VerifyToken(cookie.Value)
}

// --- Utility functions ported from src/utils.ts ---

// HashToken computes SHA-256 hex digest of the input string,
// matching the TypeScript hashToken function.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// isValidEmail validates email format, matching the TypeScript isValidEmail.
var emailRegex = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func isValidEmail(email string) bool {
	return emailRegex.MatchString(email)
}

// --- SanitizePlayerName ported from src/validators.ts ---

// sanitizePlayerName cleans a player name: removes control chars,
// zero-width chars, trims, limits length, strips dangerous HTML chars,
// and collapses whitespace. Matches the TypeScript version.
// 委托给 validate.Nickname 统一实现，消除重复代码。
func sanitizePlayerName(raw string) string {
	return validate.Nickname(raw)
}

// generateJTI creates a cryptographically random JWT ID (jti) using 16 bytes
// of randomness encoded as hex (32 characters).
func generateJTI() (string, error) {
	b := make([]byte, 16)
	if _, err := randRead(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
