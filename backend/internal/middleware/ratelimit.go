package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/requestctx"
)

// InitRateLimits overrides default rate limit config from environment variables.
// Must be called once during server initialization, before any rate-limited routes are used.
// Replaces the previous init()-based approach for testability and explicit ordering.
func InitRateLimits(requests int, window time.Duration) {
	rateLimitMu.Lock()
	defer rateLimitMu.Unlock()
	if requests > 0 {
		cfg := DefaultEndpointRateLimits["default"]
		cfg.Requests = requests
		DefaultEndpointRateLimits["default"] = cfg
	}
	if window > 0 {
		cfg := DefaultEndpointRateLimits["default"]
		cfg.Window = window
		DefaultEndpointRateLimits["default"] = cfg
	}
}

// setRateLimitHeaders writes the RFC 6585 Retry-After header and the optional
// X-RateLimit-Limit header before a 429 response is emitted.
//
// RFC 6585 §4 requires Retry-After on 429 responses.
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
// Per-user rate limiting prevents single-bad-actor exhaustion;
// per-IP rate limiting mitigates distributed attacks.
type EndpointRateLimitConfig struct {
	Requests   int
	Window     time.Duration
	FailClosed bool // if true, reject requests when Redis is unavailable
}

// DefaultEndpointRateLimits defines per-endpoint rate limits.
// Security-critical endpoints (auth:*, admin:login) use FailClosed=true
// so that Redis unavailability blocks requests rather than allowing unbounded access.
// handler-014: The "default" config is also fail-closed so that any unlisted
// endpoint rejects requests when Redis is unavailable (safer default).
var rateLimitMu sync.RWMutex

// DefaultEndpointRateLimits defines per-endpoint rate limit configurations.
var DefaultEndpointRateLimits = map[string]EndpointRateLimitConfig{
	"auth:quickplay":    {Requests: 10, Window: time.Minute, FailClosed: true},
	"auth:request":      {Requests: 5, Window: time.Minute, FailClosed: true},
	"auth:verify":       {Requests: 10, Window: time.Minute, FailClosed: true}, // handler-014: security-critical auth endpoint
	"registry:create":   {Requests: 5, Window: time.Minute},
	"registry:check":    {Requests: 30, Window: time.Minute},
	"registry:lobbies":  {Requests: 30, Window: time.Minute},
	"registry:match":    {Requests: 10, Window: time.Minute},
	"stats:leaderboard": {Requests: 60, Window: time.Minute},
	"admin:login":       {Requests: 5, Window: time.Minute, FailClosed: true},
	"default":           {Requests: 60, Window: time.Minute, FailClosed: true}, // handler-014: fail-closed default
}

// RateLimit returns middleware that checks Redis-based rate limits.
// It uses the client IP from X-Forwarded-For or RemoteAddr as the key.
//
// When Redis errors occur, behavior is controlled by config.FailClosed (v2-R-05):
//   - FailClosed=false (default): allow requests (fail-open) to avoid blocking all users during brief Redis outages
//   - FailClosed=true: reject requests (fail-closed) for security-sensitive endpoints to prevent unbounded attacks during Redis downtime
//
// Production routes use EndpointRateLimit (which configures FailClosed per endpoint).
// This base function is primarily for testing and custom scenarios.
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
// When FailClosed is true, Redis errors cause request rejection instead of admission.
// Security-sensitive endpoints (auth:quickplay, admin:login) use fail-closed.
func EndpointRateLimit(redisStore RateLimiterStore, endpoint string, jwtMgr JWTManager) func(http.Handler) http.Handler {
	rateLimitMu.RLock()
	cfg, ok := DefaultEndpointRateLimits[endpoint]
	if !ok {
		cfg = DefaultEndpointRateLimits["default"]
	}
	rateLimitMu.RUnlock()

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
// Rate limit key with user identity hierarchy: auth context → session/quickplay cookie → IP.
// Historical note: previously read a "token" cookie that didn't exist, making user-level
// rate limiting completely ineffective.
func rateLimitKey(r *http.Request, endpoint string, jwtMgr JWTManager) string {
	ip := extractClientIP(r)

	// 1. Try auth context (set by AuthMiddleware)
	if userID, _, ok := auth.GetAuthenticatedUser(r); ok && userID != "" {
		return fmt.Sprintf("%s:%s:%s", endpoint, userID, ip)
	}

	// 2. Fallback: try "session" cookie, then "quickplay" cookie
	if jwtMgr != nil {
		for _, cookieName := range []string{"session", "quickplay"} {
			if uid, _, _, _, err := parseAuthCookie(r, cookieName, jwtMgr); err == nil && uid != "" {
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
// handler-020: Delegates to requestctx.ExtractClientIP to eliminate duplication.
// The canonical implementation lives in requestctx; this wrapper exists for
// backward compatibility with callers that import middleware.
//
// Deprecated: use requestctx.ExtractClientIP directly in new code.
func ExtractClientIP(r *http.Request) string {
	return requestctx.ExtractClientIP(r)
}
