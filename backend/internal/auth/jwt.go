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
	"github.com/uppy-clone/backend/internal/domain"
)

// randRead is injectable for unit tests (e.g. simulate crypto/rand failures).
var randRead = rand.Read

// SetRandReadHook overrides crypto/rand.Read in tests and returns a restore func.
func SetRandReadHook(fn func([]byte) (int, error)) (restore func()) {
	prev := randRead
	randRead = fn
	return func() { randRead = prev }
}

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

// customClaims extends jwt.RegisteredClaims with nickname and role.
type customClaims struct {
	Nickname string `json:"nickname"`
	Role     string `json:"role,omitempty"`
	jwt.RegisteredClaims
}

// SignToken creates a JWT with userId (sub), nickname, and jti claims.
// Access token expires in 15 minutes; use refresh tokens for longer sessions.
// 企业为何需要：无撤销机制的 JWT 意味着被盗 token 在过期前持续有效。JWT 撤销列表是登出安全的行业标准实现，
// 用 Redis SET + TTL 实现最小性能开销。jti (JWT ID) 是 RFC 7519 标准字段，用于唯一标识每个 token。
func (m *JWTManager) SignToken(userId, nickname string) (string, error) {
	return m.SignTokenWithRole(userId, nickname, domain.RoleUser)
}

// SignTokenWithRole creates a JWT with userId (sub), nickname, role, and jti claims.
// Access token expires in 15 minutes; use refresh tokens for longer sessions.
// 企业为何需要：role claim 使 AuthMiddleware 能从已验证的凭据中读取用户角色，
// 而非硬编码。这确保即使中间件误挂，RBAC 仍能基于 token 中的 role 做出正确决策。
func (m *JWTManager) SignTokenWithRole(userId, nickname, role string) (string, error) {
	now := time.Now()
	jti, err := generateJTI()
	if err != nil {
		return "", fmt.Errorf("generate jti: %w", err)
	}
	claims := customClaims{
		Nickname: nickname,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   userId,
			Issuer:    config.JWTIssuer,
			Audience:  []string{config.JWTAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(config.AccessTokenTTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	return token.SignedString(m.privateKey)
}

// SignWithClaims signs a JWT with the provided custom claims using ES256.
// This allows callers to create tokens with custom fields without exposing
// the raw ECDSA private key.
func (m *JWTManager) SignWithClaims(claims map[string]any) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims(claims))
	return token.SignedString(m.privateKey)
}

// PublicKey returns the ECDSA public key.
func (m *JWTManager) PublicKey() *ecdsa.PublicKey {
	return m.publicKey
}

type jwtParseFunc func(string, jwt.Claims, jwt.Keyfunc, ...jwt.ParserOption) (*jwt.Token, error)

// jwtParseWithClaimsFn is injectable for unit tests (e.g. invalid claims paths).
var jwtParseWithClaimsFn jwtParseFunc = jwt.ParseWithClaims

// VerifyToken validates a JWT and returns userId, nickname, jti, and role.
// If the token has no role claim (legacy tokens), role defaults to "user".
func (m *JWTManager) VerifyToken(tokenStr string) (userID, nickname, jti, role string, err error) {
	// auth-002: Verify Issuer and Audience to prevent token confusion across services.
	token, err := jwtParseWithClaimsFn(tokenStr, &customClaims{}, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodES256 {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.publicKey, nil
	}, jwt.WithIssuer(config.JWTIssuer), jwt.WithAudience(config.JWTAudience))
	if err != nil {
		return "", "", "", "", fmt.Errorf("verify token: %w", err)
	}

	claims, ok := token.Claims.(*customClaims)
	if !ok || !token.Valid {
		return "", "", "", "", fmt.Errorf("invalid token claims")
	}

	role = claims.Role
	if role == "" {
		role = domain.RoleUser
	}
	return claims.Subject, claims.Nickname, claims.ID, role, nil
}

// BuildAuthCookie creates an HttpOnly, SameSite=Lax, Secure cookie
// matching the TypeScript buildAuthCookie behavior.
func BuildAuthCookie(name, value string, maxAge int, secure bool) *http.Cookie {
	return &http.Cookie{ //nolint:gosec // G124: Secure flag is conditional to support local dev without TLS
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
func ParseAuthCookie(r *http.Request, cookieName string, jwtManager *JWTManager) (userId, nickname, jti, role string, err error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return "", "", "", "", fmt.Errorf("cookie %s not found: %w", cookieName, err)
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
// 委托给 domain.SanitizeNickname 统一实现，消除重复代码。
func sanitizePlayerName(raw string) string {
	return domain.SanitizeNickname(raw)
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
