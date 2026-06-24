package auth

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/slogctx"
)

type contextKey string

const (
	userIDKey   contextKey = "auth_user_id"
	nicknameKey contextKey = "auth_nickname"
	roleKey     contextKey = "auth_user_role"
	jtiKey      contextKey = "auth_jti"
)

// JWTRevocationChecker checks if a JWT has been revoked by its jti.
// 企业为何需要：无撤销机制的 JWT 意味着被盗 token 在过期前持续有效。JWT 撤销列表是登出安全的行业标准实现，
// 用 Redis SET + TTL 实现最小性能开销。
type JWTRevocationChecker interface {
	IsJWTRevoked(ctx context.Context, jti string) (bool, error)
}

// redisClientProvider is optionally implemented by the revoker (e.g. *store.RedisStore)
// to expose a Redis client for multi-IP anomaly detection.
// 企业为何需要：避免 auth 包直接依赖 store 包（防止循环导入），通过接口反转依赖。
type redisClientProvider interface {
	Client() *redis.Client
}

// maxIPsPerHour is the threshold for suspicious multi-IP login detection.
// 企业为何需要：单用户 1 小时内超过 3 个不同 IP 表明凭证可能被盗或共享，需告警。
const maxIPsPerHour = 3

// tryCookie attempts to read and validate a JWT from the named cookie.
// Returns userID, nickname, jti if valid, empty strings otherwise.
// Extracted to eliminate duplicated cookie-reading code in AuthMiddleware.
func tryCookie(r *http.Request, jwtMgr *JWTManager, name string) (userID, nickname, jti string) {
	cookie, err := r.Cookie(name)
	if err != nil || cookie.Value == "" {
		return "", "", ""
	}
	uid, nick, j, err := jwtMgr.VerifyToken(cookie.Value)
	if err != nil {
		return "", "", ""
	}
	return uid, nick, j
}

// AuthMiddleware checks for valid JWT in quickplay or session cookie.
// It tries "session" cookie first, then "quickplay" cookie.
// If valid, injects userId, nickname, and jti into request context.
// If neither cookie is valid, returns 401.
// If revoker is non-nil, also checks if the JWT's jti has been revoked.
func AuthMiddleware(jwtMgr *JWTManager, next http.HandlerFunc, revoker ...JWTRevocationChecker) http.HandlerFunc {
	var rev JWTRevocationChecker
	if len(revoker) > 0 {
		rev = revoker[0]
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// Try "session" cookie first (Magic Link login), then "quickplay" cookie
		for _, cookieName := range []string{"session", "quickplay"} {
			userId, nickname, jti := tryCookie(r, jwtMgr, cookieName)
			if userId == "" {
				continue
			}
			if rev != nil {
				revoked, revErr := rev.IsJWTRevoked(r.Context(), jti)
				if revErr != nil {
					slog.Error("jwt revocation check failed", "error", revErr)
					apierror.Unauthorized("Unauthorized").Write(w)
					return
				}
				if revoked {
					slog.Info("revoked jwt used", "jti", jti)
					apierror.Unauthorized("Unauthorized").Write(w)
					return
				}
			}
			ctx := context.WithValue(r.Context(), userIDKey, userId)
			ctx = context.WithValue(ctx, nicknameKey, nickname)
			ctx = context.WithValue(ctx, jtiKey, jti)
			// 企业为何需要：普通用户 JWT 不含 role claim，统一标记为 "user"。
			// 角色必须来自已验证的凭据，而非客户端可控的 HTTP 头。
			ctx = WithRole(ctx, "user")
			// Multi-IP anomaly detection — track distinct IPs per user.
			// 企业为何需要：多 IP 同账户登录是账户盗用/凭证共享的典型信号，需告警。
			if rev != nil {
				if provider, ok := rev.(redisClientProvider); ok {
					detectMultiIPLogin(r.Context(), provider.Client(), userId, clientIPFromRequest(r))
				}
			}
			// Inject user_id into slog context for structured logging
			if logger := slogctx.LoggerFromContext(ctx); logger != nil {
				logger = logger.With("user_id", userId)
				ctx = slogctx.WithLogger(ctx, logger)
			}
			next(w, r.WithContext(ctx))
			return
		}

		// Neither cookie is valid
		apierror.Unauthorized("Unauthorized").Write(w)
	}
}

// GetAuthenticatedUser extracts user info from request context (set by AuthMiddleware).
// Returns userId, nickname, and ok=true if authenticated.
func GetAuthenticatedUser(r *http.Request) (userId, nickname string, ok bool) {
	uid, ok1 := r.Context().Value(userIDKey).(string)
	nick, ok2 := r.Context().Value(nicknameKey).(string)
	if !ok1 || !ok2 || uid == "" {
		return "", "", false
	}
	return uid, nick, true
}

// GetJTI extracts the JWT ID from request context (set by AuthMiddleware).
func GetJTI(r *http.Request) string {
	if jti, ok := r.Context().Value(jtiKey).(string); ok {
		return jti
	}
	return ""
}

// WithJTI returns a new context with the given jti value set.
// Used by adminAuthMiddleware to inject the admin token's jti so that
// Logout and password-change handlers can revoke it.
func WithJTI(ctx context.Context, jti string) context.Context {
	return context.WithValue(ctx, jtiKey, jti)
}

// WithAuthenticatedUser returns a new context with userId and nickname values set.
// This is the inverse of GetAuthenticatedUser and is primarily used in tests.
func WithAuthenticatedUser(ctx context.Context, userId, nickname string) context.Context {
	ctx = context.WithValue(ctx, userIDKey, userId)
	ctx = context.WithValue(ctx, nicknameKey, nickname)
	return ctx
}

// WithRole returns a new context with the given role value.
// 企业为何需要：角色必须来自已验证的凭据（JWT claims/已通过认证的中间件），
// 而非客户端可控的 HTTP 头。X-User-Role 头可被任意客户端伪造，导致权限提升。
// 该函数供认证中间件在验证凭据后注入角色，RBAC 中间件再从 context 读取。
func WithRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, roleKey, role)
}

// RoleFromContext extracts the role from the request context (set by auth middleware).
// Returns the role and ok=true if a role was set by an authenticated middleware.
func RoleFromContext(r *http.Request) (string, bool) {
	role, ok := r.Context().Value(roleKey).(string)
	return role, ok
}

// clientIPFromRequest extracts the client IP from the request, handling
// proxied environments by trusting RemoteAddr (set by RealIP middleware).
func clientIPFromRequest(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// detectMultiIPLogin tracks the client IP for a user in a Redis Set with a
// 1-hour TTL. If the user has logged in from more than maxIPsPerHour distinct
// IPs within the window, it increments the suspicious_login_total counter and
// logs a warning.
// 企业为何需要：多 IP 同账户登录是账户盗用的典型信号，需可观测并告警。
func detectMultiIPLogin(ctx context.Context, rdb *redis.Client, userID, clientIP string) {
	if rdb == nil || userID == "" || clientIP == "" {
		return
	}

	ipKey := "user:ips:" + userID
	// Add current IP to the set with a 1-hour TTL.
	if err := rdb.SAdd(ctx, ipKey, clientIP).Err(); err != nil {
		slog.Warn("failed to track user IP", "user_id", userID, "error", err)
		return
	}
	rdb.Expire(ctx, ipKey, time.Hour)

	// Count distinct IPs in the window.
	ipCount, err := rdb.SCard(ctx, ipKey).Result()
	if err != nil {
		slog.Warn("failed to count user IPs", "user_id", userID, "error", err)
		return
	}

	if ipCount > int64(maxIPsPerHour) {
		metrics.SuspiciousLoginTotal.Inc()
		slog.Warn("suspicious: multiple IPs for user",
			"user_id", userID, "ip_count", ipCount, "current_ip", clientIP)
	}
}
