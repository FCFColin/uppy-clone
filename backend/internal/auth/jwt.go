package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/validate"
)

// JWTManager handles JWT signing and verification using ECDSA P-256 (ES256).
type JWTManager struct {
	privateKey *ecdsa.PrivateKey
	publicKey  *ecdsa.PublicKey
}

// NewJWTManager creates a JWTManager from PEM-encoded ECDSA P-256 keys.
// If privateKeyPEM is empty, an ephemeral key pair is generated for dev.
func NewJWTManager(privateKeyPEM string) *JWTManager {
	if privateKeyPEM == "" {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			panic(fmt.Sprintf("generate ephemeral ECDSA key: %v", err))
		}
		return &JWTManager{privateKey: key, publicKey: &key.PublicKey}
	}
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		panic("JWT_PRIVATE_KEY: failed to decode PEM block")
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		panic(fmt.Sprintf("JWT_PRIVATE_KEY: parse ECDSA private key: %v", err))
	}
	return &JWTManager{privateKey: key, publicKey: &key.PublicKey}
}

// NewJWTManagerWithKeys creates a JWTManager from parsed ECDSA keys directly.
func NewJWTManagerWithKeys(privateKey *ecdsa.PrivateKey, publicKey *ecdsa.PublicKey) *JWTManager {
	return &JWTManager{privateKey: privateKey, publicKey: publicKey}
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
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	return token.SignedString(m.privateKey)
}

// PrivateKey returns the ECDSA private key.
func (m *JWTManager) PrivateKey() *ecdsa.PrivateKey {
	return m.privateKey
}

// PublicKey returns the ECDSA public key.
func (m *JWTManager) PublicKey() *ecdsa.PublicKey {
	return m.publicKey
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
