package main

import (
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/handler"
	appMiddleware "github.com/uppy-clone/backend/internal/middleware"
	"github.com/uppy-clone/backend/internal/rbac"
	"github.com/uppy-clone/backend/internal/store"
)

// setupMiddleware registers global chi middleware (logging, recovery, security, CORS).
func setupMiddleware(r *chi.Mux) {
	r.Use(chiMiddleware.Logger)
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.RequestID)
	r.Use(appMiddleware.RequestIDLogger)
	r.Use(appMiddleware.TracingMiddleware)
	r.Use(appMiddleware.PrometheusMiddleware)
	r.Use(appMiddleware.TrustedProxy(getEnv("TRUSTED_PROXY_CIDRS", "")))
	r.Use(appMiddleware.SecurityHeaders)

	// CORS
	allowedOrigins := appMiddleware.AllowedOriginsFromEnv(getEnv("ALLOWED_ORIGINS", ""))
	r.Use(appMiddleware.CORS(allowedOrigins))
}

// metricsAuthMiddleware wraps a handler with Basic Auth for /metrics endpoint.
func metricsAuthMiddleware(next http.Handler) http.Handler {
	user := os.Getenv("METRICS_USER")
	pass := os.Getenv("METRICS_PASSWORD")
	production := os.Getenv("ENABLE_HSTS") != "false"
	if production && (user == "" || pass == "") {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

// authMiddlewareWrapper wraps auth.AuthMiddleware to work as chi middleware.
func authMiddlewareWrapper(jwtMgr *auth.JWTManager, redis *store.RedisStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth.AuthMiddleware(jwtMgr, func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r)
			}, redis)(w, r)
		})
	}
}

// adminAuthMiddleware checks for a valid admin JWT cookie.
// 企业为何需要：角色必须来自已验证的凭据（JWT claims），而非客户端可控输入。
// 同时将 token 的 jti 注入 context，供 Logout 和改密撤销流程使用。
func adminAuthMiddleware(adminHandler *handler.AdminHandler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := adminHandler.VerifyAdminTokenClaims(r)
			if !ok {
				apierror.Unauthorized("Unauthorized").Write(w)
				return
			}
			ctx := auth.WithRole(r.Context(), rbac.RoleAdmin)
			ctx = auth.WithJTI(ctx, claims.ID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
