package server

import (
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/handler"
	appMiddleware "github.com/uppy-clone/backend/internal/middleware"
	"github.com/uppy-clone/backend/internal/store"
)

// setupMiddleware registers global chi middleware (logging, recovery, security, CORS).
// Note: chiMiddleware.Logger and chiMiddleware.Recoverer are intentionally omitted —
// appMiddleware.RequestIDLogger covers structured request logging (with RequestID),
// and appMiddleware.Recovery covers panic recovery with slog. The chi built-ins
// were redundant and added noise without RequestID correlation.
func setupMiddleware(r *chi.Mux) {
	r.Use(chiMiddleware.RequestID)

	allowedOrigins := appMiddleware.AllowedOriginsFromEnv(serverEnv.AllowedOrigins)
	r.Use(appMiddleware.CORS(allowedOrigins))

	r.Use(appMiddleware.Recovery)
	r.Use(appMiddleware.RequestIDLogger)
	r.Use(appMiddleware.TracingMiddleware)
	r.Use(appMiddleware.PrometheusMiddleware)
	r.Use(appMiddleware.TrustedProxy(serverEnv.TrustedProxyCIDRs))
	r.Use(appMiddleware.SecurityHeaders)
}

// metricsAuthMiddleware wraps a handler with Basic Auth for /metrics endpoint.
func metricsAuthMiddleware(next http.Handler) http.Handler {
	user := os.Getenv("METRICS_USER")
	pass := os.Getenv("METRICS_PASSWORD")
	production := os.Getenv("ENV") == "production"
	if production && (user == "" || pass == "") {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "Metrics endpoint disabled: configure METRICS_USER and METRICS_PASSWORD", http.StatusForbidden)
		})
	}
	if user == "" || pass == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != user || p != pass {
			w.Header().Set("WWW-Authenticate", `Basic realm="metrics"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// authMiddlewareWrapper wraps appMiddleware.AuthMiddleware to work as chi middleware.
func authMiddlewareWrapper(jwtMgr *auth.JWTManager, redis *store.RedisStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			appMiddleware.AuthMiddleware(jwtMgr, func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r)
			}, redis)(w, r)
		})
	}
}

// adminAuthMiddleware checks for a valid admin JWT cookie.
// Roles come from verified credentials (JWT claims), not client-controlled input.
// The token jti is injected into context for logout/password-change revocation.
func adminAuthMiddleware(adminHandler *handler.AdminHandler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := adminHandler.VerifyAdminTokenClaims(r)
			if !ok {
				apierror.Unauthorized("Unauthorized").Write(w)
				return
			}
			ctx := domain.WithRole(r.Context(), domain.RoleAdmin)
			ctx = auth.WithJTI(ctx, claims.ID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
