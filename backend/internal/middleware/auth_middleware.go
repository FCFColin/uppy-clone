package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/util"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var authTracer = otel.Tracer("github.com/uppy-clone/backend/internal/middleware")

// maxIPsPerHour is the threshold for suspicious multi-IP login detection.
const maxIPsPerHour = 3

const (
	sessionCookie   = "session"
	quickplayCookie = "quickplay"
)

// tryCookie attempts to read and validate a JWT from the named cookie.
// Returns userID, nickname, jti, role if valid, empty strings otherwise.
func tryCookie(r *http.Request, jwtMgr *auth.JWTManager, name string) (userID, nickname, jti, role string) {
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

// AuthMiddleware checks for valid JWT in quickplay or session cookie.
// It tries "session" cookie first, then "quickplay" cookie.
// If valid, injects userId, nickname, and jti into request context.
// If neither cookie is valid, returns 401.
// If revoker is non-nil, also checks if the JWT's jti has been revoked.
func AuthMiddleware(jwtMgr *auth.JWTManager, next http.HandlerFunc, revoker ...auth.JWTRevocationChecker) http.HandlerFunc {
	var rev auth.JWTRevocationChecker
	if len(revoker) > 0 {
		rev = revoker[0]
	}

	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := authTracer.Start(r.Context(), "middleware.auth")
		defer span.End()
		r = r.WithContext(ctx)

		// Try "session" cookie first (Magic Link login), then "quickplay" cookie
		for _, cookieName := range []string{sessionCookie, quickplayCookie} {
			userId, nickname, jti, role := tryCookie(r, jwtMgr, cookieName)
			if userId == "" {
				continue
			}
			if rev != nil {
				_, revSpan := authTracer.Start(ctx, "middleware.revocation_check")
				revoked, revErr := rev.IsJWTRevoked(ctx, jti)
				if revErr != nil {
					slog.Error("jwt revocation check failed", "error", revErr)
					revSpan.End()
					domain.Unauthorized("Unauthorized").Write(w)
					return
				}
				if revoked {
					slog.Info("revoked jwt used", "jti", jti)
					revSpan.SetAttributes(attribute.Bool("revoked", true))
					revSpan.End()
					domain.Unauthorized("Unauthorized").Write(w)
					return
				}
				revSpan.End()
			}
			ctx = context.WithValue(r.Context(), domain.ContextKeyUserID, userId)
			ctx = context.WithValue(ctx, domain.ContextKeyNickname, nickname)
			ctx = context.WithValue(ctx, domain.ContextKeyJTI, jti)
			ctx = domain.WithRole(ctx, role)
			// Multi-IP anomaly detection — track distinct IPs per user.
			if rev != nil {
				if scripter, ok := rev.(redis.Scripter); ok {
					_, ipSpan := authTracer.Start(ctx, "middleware.multi_ip_detection")
					detectMultiIPLogin(ctx, scripter, userId, ExtractClientIP(r))
					ipSpan.End()
				}
			}
			// Inject user_id and role into slog context for structured logging
			if logger := util.LoggerFromContext(ctx); logger != nil {
				ctx = util.WithLogger(ctx, logger.With("user_id", userId, "role", role))
			}
			next(w, r.WithContext(ctx))
			return
		}

		// Neither cookie is valid
		span.SetAttributes(attribute.Bool("authenticated", false))
		domain.Unauthorized("Unauthorized").Write(w)
	}
}

// detectMultiIPLogin tracks the client IP for a user in a Redis Set with a
// 1-hour TTL. If the user has logged in from more than maxIPsPerHour distinct
// IPs within the window, it increments the suspicious_login_total counter and
// logs a warning.
func detectMultiIPLogin(ctx context.Context, rdb redis.Scripter, userID, clientIP string) {
	if rdb == nil || userID == "" || clientIP == "" {
		return
	}

	ipCount, err := store.TrackUserIPs(ctx, rdb, userID, clientIP)
	if err != nil {
		slog.Warn("failed to track user IP", "user_id", userID, "error", err)
		return
	}

	if ipCount > int64(maxIPsPerHour) {
		metrics.SuspiciousLoginTotal.Inc()
		slog.Warn("suspicious: multiple IPs for user",
			"user_id", userID, "ip_count", ipCount, "current_ip", clientIP)
	}
}

// parseAuthCookie reads the named cookie and verifies it with the given token verifier.
//
// RO-051 (interface segregation): the parameter is auth.TokenVerifier rather
// than *auth.JWTManager so that middleware depends on the narrow capability
// it needs (VerifyToken) and test doubles can be injected without spinning up
// a real JWTManager. *auth.JWTManager already satisfies auth.TokenVerifier.
func parseAuthCookie(r *http.Request, cookieName string, verifier auth.TokenVerifier) (userID, nickname, jti, role string, err error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return "", "", "", "", fmt.Errorf("cookie %s not found: %w", cookieName, err)
	}
	return verifier.VerifyToken(cookie.Value)
}
