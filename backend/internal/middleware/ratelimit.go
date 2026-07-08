package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/auth"
)

func init() {
	if v := os.Getenv("RATE_LIMIT_DEFAULT_REQUESTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg := DefaultEndpointRateLimits["default"]
			cfg.Requests = n
			DefaultEndpointRateLimits["default"] = cfg
		}
	}
	if v := os.Getenv("RATE_LIMIT_DEFAULT_WINDOW_SECS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg := DefaultEndpointRateLimits["default"]
			cfg.Window = time.Duration(n) * time.Second
			DefaultEndpointRateLimits["default"] = cfg
		}
	}
}

// setRateLimitHeaders writes the RFC 6585 Retry-After header and the optional
// X-RateLimit-Limit header before a 429 response is emitted.
//
// 企业为何需要：RFC 6585 §4 要求 429 响应携带 Retry-After，告知客户端何时可重试，
// 否则客户端只能盲目退避，加剧拥塞或延长用户感知延迟。X-RateLimit-Limit 让客户端
// 能自适应配额。X-RateLimit-Remaining/Reset 需要存储层返回，当前 store 接口仅
// 返回 (bool, error)，故暂不设置，避免过度工程化。
func setRateLimitHeaders(w http.ResponseWriter, limit int, window time.Duration) {
	w.Header().Set("Retry-After", strconv.Itoa(int(window.Seconds())))
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
}

// RateLimiterStore abstracts the rate-limit check so the middleware can be
// unit-tested with a fake store. *store.RedisStore satisfies this interface
// via its CheckRateLimit method.
type RateLimiterStore interface {
	CheckRateLimit(ctx context.Context, key string, maxCount int64, window time.Duration) (bool, error)
}

// RateLimitConfig holds rate limit configuration.
type RateLimitConfig struct {
	MaxRequests int64
	Window      time.Duration
	FailClosed  bool // if true, reject requests when Redis is unavailable (v2-R-05)
}

// EndpointRateLimitConfig defines per-endpoint rate limits.
// 企业为何需要：限流是 DoS 防御的基础设施。按用户维度限流防止单一恶意用户耗尽配额，
// 而按 IP 限制防止分布式攻击。两者缺一不可。
type EndpointRateLimitConfig struct {
	Requests   int
	Window     time.Duration
	FailClosed bool // if true, reject requests when Redis is unavailable
}

// DefaultEndpointRateLimits defines per-endpoint rate limits.
// Security-critical endpoints (auth:quickplay, admin:login) use FailClosed=true
// so that Redis unavailability blocks requests rather than allowing unbounded access.
var DefaultEndpointRateLimits = map[string]EndpointRateLimitConfig{
	"auth:quickplay":    {Requests: 10, Window: time.Minute, FailClosed: true},
	"auth:request":      {Requests: 5, Window: time.Minute},
	"auth:verify":       {Requests: 10, Window: time.Minute},
	"registry:create":   {Requests: 5, Window: time.Minute},
	"registry:check":    {Requests: 30, Window: time.Minute},
	"registry:lobbies":  {Requests: 30, Window: time.Minute},
	"registry:match":    {Requests: 10, Window: time.Minute},
	"stats:leaderboard": {Requests: 60, Window: time.Minute},
	"admin:login":       {Requests: 5, Window: time.Minute, FailClosed: true},
	"default":           {Requests: 60, Window: time.Minute},
}

// RateLimit returns middleware that checks Redis-based rate limits.
// It uses the client IP from X-Forwarded-For or RemoteAddr as the key.
//
// 当 Redis 出错时，行为由 config.FailClosed 控制（v2-R-05）：
//   - FailClosed=false（默认）：放行请求（fail-open），避免 Redis 短暂故障阻断全部用户
//   - FailClosed=true：拒绝请求（fail-closed），用于安全敏感端点防止 Redis 宕机时无限制攻击
//
// 生产路由使用 EndpointRateLimit（按端点配置 FailClosed），此基础函数主要供测试和
// 自定义场景使用。
func RateLimit(redisStore RateLimiterStore, config RateLimitConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := extractClientIP(r)
			key := clientIP

			allowed, err := redisStore.CheckRateLimit(r.Context(), key, config.MaxRequests, config.Window)
			if err != nil {
				if config.FailClosed {
					setRateLimitHeaders(w, int(config.MaxRequests), config.Window)
					apierror.TooManyRequests("Service temporarily unavailable. Please try again later.").Write(w)
					return
				}
				// Fail-open: allow the request through on Redis error
				next.ServeHTTP(w, r)
				return
			}

			if !allowed {
				setRateLimitHeaders(w, int(config.MaxRequests), config.Window)
				apierror.TooManyRequests("Too many requests. Please try again later.").Write(w)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// EndpointRateLimit returns middleware that checks Redis-based rate limits
// using a composite key of endpoint + user ID (from auth context or cookies) + IP.
//
// jwtMgr is used to extract userID from "session" or "quickplay" cookies when
// the auth middleware has not set the context (e.g., on unauthenticated endpoints).
// It may be nil, in which case cookie-based fallback is skipped.
//
// Fail-closed 策略：当配置的 EndpointRateLimitConfig.FailClosed 为 true 时，
// Redis 出错将拒绝请求而非放行。安全敏感端点（auth:quickplay、admin:login）
// 应使用 fail-closed，防止 Redis 宕机时遭受无限制攻击。
func EndpointRateLimit(redisStore RateLimiterStore, endpoint string, jwtMgr JWTManager) func(http.Handler) http.Handler {
	cfg, ok := DefaultEndpointRateLimits[endpoint]
	if !ok {
		cfg = DefaultEndpointRateLimits["default"]
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := rateLimitKey(r, endpoint, jwtMgr)

			allowed, err := redisStore.CheckRateLimit(r.Context(), key, int64(cfg.Requests), cfg.Window)
			if err != nil {
				if cfg.FailClosed {
					// Security-critical endpoint: reject on Redis failure
					setRateLimitHeaders(w, cfg.Requests, cfg.Window)
					apierror.TooManyRequests("Service temporarily unavailable. Please try again later.").Write(w)
					return
				}
				// Non-critical endpoint: allow on Redis failure (fail-open)
				next.ServeHTTP(w, r)
				return
			}

			if !allowed {
				setRateLimitHeaders(w, cfg.Requests, cfg.Window)
				apierror.TooManyRequests("Too many requests. Please try again later.").Write(w)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// rateLimitKey builds a composite rate limit key from endpoint, user ID, and IP.
//
// 企业为何需要：限流维度必须正确识别用户，否则认证用户可绕过限流。
// 优先从 auth 中间件已注入 context 的 user_id 读取；若 context 无值（如
// 未经 AuthMiddleware 的端点），则回退到直接解析 "session" 或 "quickplay"
// cookie 提取 userID。两者均无则回退到 IP 维度。
//
// 历史缺陷：此前读取名为 "token" 的 cookie，但实际认证 cookie 名为
// "session" 与 "quickplay"（见 auth.BuildAuthCookie / auth.AuthMiddleware），
// 导致 userID 恒为空，用户级限流完全失效。
func rateLimitKey(r *http.Request, endpoint string, jwtMgr JWTManager) string {
	ip := extractClientIP(r)

	// 1. Try auth context (set by AuthMiddleware)
	if userID, _, ok := auth.GetAuthenticatedUser(r); ok && userID != "" {
		return fmt.Sprintf("%s:%s:%s", endpoint, userID, ip)
	}

	// 2. Fallback: try "session" cookie, then "quickplay" cookie
	if jwtMgr != nil {
		for _, cookieName := range []string{"session", "quickplay"} {
			if uid, _, _, err := parseAuthCookie(r, cookieName, jwtMgr); err == nil && uid != "" {
				return fmt.Sprintf("%s:%s:%s", endpoint, uid, ip)
			}
		}
	}

	// 3. No user identity available — IP-only key
	return fmt.Sprintf("%s:%s", endpoint, ip)
}

// extractClientIP returns the client IP from X-Forwarded-For or RemoteAddr.
func extractClientIP(r *http.Request) string {
	return ExtractClientIP(r)
}

// ExtractClientIP returns the client IP from X-Forwarded-For or RemoteAddr.
// Exported so that other packages (e.g., handler/admin.go) can reuse the
// same reverse-proxy-aware IP extraction logic for lockout keys.
//
// 反向代理（GKE Ingress/nginx）后，r.RemoteAddr 为代理地址，须解析 X-Forwarded-For。
// 恒为代理 IP，所有攻击者共享同一锁定 key，导致 DoS（单攻击者锁全部用户）
// 或暴力破解成功（锁定永不触发）。必须从 X-Forwarded-For 提取真实客户端 IP。
func ExtractClientIP(r *http.Request) string {
	// Only trust X-Forwarded-For from configured reverse proxies.
	if !IsTrustedProxy(r) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			return r.RemoteAddr
		}
		return ip
	}

	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// X-Forwarded-For may contain multiple IPs; use the first one
		ips := strings.SplitN(xff, ",", 2)
		ip := strings.TrimSpace(ips[0])
		if ip != "" {
			return ip
		}
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
