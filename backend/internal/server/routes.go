// Package server wires HTTP routes, middleware, and the application lifecycle.
package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker/v2"

	"github.com/uppy-clone/backend/internal/auth"
	appConfig "github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/handler"
	"github.com/uppy-clone/backend/internal/health"
	appMiddleware "github.com/uppy-clone/backend/internal/middleware"
	"github.com/uppy-clone/backend/internal/rbac"
	"github.com/uppy-clone/backend/internal/store"
)

// setupRoutes registers all HTTP routes and middleware on the chi router.
func setupRoutes(r *chi.Mux, authHandler *handler.AuthHandler, lobbyHandler *handler.LobbyHandler, adminHandler *handler.AdminHandler, statsHandler *handler.StatsHandler, jwtMgr *auth.JWTManager, db *store.PostgresStore, cluster *store.RedisCluster, rbacEnforcer *rbac.Enforcer, cfg *handler.Config, hub *game.Hub) {
	setupMiddleware(r)
	setupHealthAndMetricsRoutes(r, db, cluster, hub)
	setupAuthRoutes(r, authHandler, cluster, jwtMgr, rbacEnforcer)
	setupStatsRoutes(r, statsHandler, cluster, jwtMgr, rbacEnforcer)
	setupLobbyRoutes(r, lobbyHandler, cluster, jwtMgr, rbacEnforcer)
	setupAdminRoutes(r, adminHandler, cluster, jwtMgr, rbacEnforcer)
	setupStaticRoutes(r, cfg)
}

// setupHealthAndMetricsRoutes registers health probes and the metrics endpoint.
func setupHealthAndMetricsRoutes(r *chi.Mux, db *store.PostgresStore, cluster *store.RedisCluster, hub *game.Hub) {
	// P2-10: readiness 探测纳入 WebSocket 舱壁负载，连接达上限时返回 503，
	// 避免流量继续打入无法承载的实例。
	var pool *pgxpool.Pool
	if db != nil {
		pool = db.Pool()
	}
	var redisPinger *redis.Client
	if cluster != nil {
		redisPinger = cluster.Stateful.Client()
	}
	var cbs []*gobreaker.CircuitBreaker[any]
	if db != nil {
		cbs = append(cbs, db.CircuitBreaker())
	}
	if cluster != nil {
		cbs = append(cbs, cluster.CircuitBreakers()...)
	}

	healthChecker := health.NewChecker(pool, redisPinger)
	if hub != nil {
		healthChecker = healthChecker.WithCanAcceptWS(hub.CanAcceptWSConnection)
	}
	healthChecker = healthChecker.WithCircuitBreakers(cbs...)
	r.Get("/health/live", healthChecker.LiveHandler)
	r.Get("/health/ready", healthChecker.ReadyHandler)
	r.Get("/health", healthChecker.ReadyHandler)

	r.Handle("/metrics", metricsAuthMiddleware(promhttp.Handler()))
	r.Get("/health/degraded", handler.DegradedHandler(cbs...))
}

// setupAuthRoutes registers auth and user-data (GDPR) routes.
// Rate limiting uses the ephemeral Redis (ADR-029); auth/session uses stateful Redis.
func setupAuthRoutes(r *chi.Mux, authHandler *handler.AuthHandler, cluster *store.RedisCluster, jwtMgr *auth.JWTManager, rbacEnforcer *rbac.Enforcer) {
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.With(appMiddleware.EndpointRateLimit(cluster.Ephemeral, appMiddleware.EndpointAuthQuickplay, jwtMgr), appMiddleware.RecordAuthMetrics("quickplay")).Post("/quickplay", authHandler.QuickPlay)
		r.With(appMiddleware.RecordAuthMetrics("check")).Get("/check", authHandler.CheckAuth)
		r.With(appMiddleware.EndpointRateLimit(cluster.Ephemeral, "auth:refresh", jwtMgr), appMiddleware.RecordAuthMetrics("refresh")).Post("/refresh", authHandler.RefreshToken)
		r.With(appMiddleware.RecordAuthMetrics("logout")).Post("/logout", authHandler.Logout)
	})

	r.Route("/api/v1/user", func(r chi.Router) {
		r.With(appMiddleware.EndpointRateLimit(cluster.Ephemeral, "user:data", jwtMgr), authMiddlewareWrapper(jwtMgr, cluster.Stateful), rbacEnforcer.Middleware("user_data", "read")).Get("/data", authHandler.ExportUserData)
		r.With(appMiddleware.EndpointRateLimit(cluster.Ephemeral, "user:data", jwtMgr), authMiddlewareWrapper(jwtMgr, cluster.Stateful), rbacEnforcer.Middleware("user_data", "delete")).Delete("/data", authHandler.DeleteUserData)
	})
}

// setupStatsRoutes registers leaderboard and user stats routes.
// Rate limiting uses ephemeral Redis (ADR-029).
func setupStatsRoutes(r *chi.Mux, statsHandler *handler.StatsHandler, cluster *store.RedisCluster, jwtMgr *auth.JWTManager, rbacEnforcer *rbac.Enforcer) {
	if statsHandler == nil {
		return
	}
	// Public stats — unauthenticated, shown on the landing page.
	r.With(appMiddleware.EndpointRateLimit(cluster.Ephemeral, "stats:public", jwtMgr)).Get("/api/v1/stats/public", statsHandler.GetPublicStats)
	r.With(appMiddleware.EndpointRateLimit(cluster.Ephemeral, "stats:leaderboard", jwtMgr)).Get("/api/v1/leaderboard", statsHandler.GetLeaderboard)
	r.With(
		appMiddleware.EndpointRateLimit(cluster.Ephemeral, "user:stats", jwtMgr),
		authMiddlewareWrapper(jwtMgr, cluster.Stateful),
		rbacEnforcer.Middleware("user_data", "read"),
	).Get("/api/v1/user/stats", statsHandler.GetUserStats)
}

// setupLobbyRoutes registers registry (room create/check/list) and lobby WebSocket routes.
// Rate limiting uses ephemeral Redis; auth/session uses stateful Redis (ADR-029).
func setupLobbyRoutes(r *chi.Mux, lobbyHandler *handler.LobbyHandler, cluster *store.RedisCluster, jwtMgr *auth.JWTManager, rbacEnforcer *rbac.Enforcer) {
	r.Route("/api/v1/registry", func(r chi.Router) {
		r.With(
			appMiddleware.EndpointRateLimit(cluster.Ephemeral, appMiddleware.EndpointRegistryCreate, jwtMgr),
			authMiddlewareWrapper(jwtMgr, cluster.Stateful),
			rbacEnforcer.Middleware("lobby", "create"),
		).Post("/create", lobbyHandler.CreateRoom)

		r.With(
			appMiddleware.EndpointRateLimit(cluster.Ephemeral, "registry:check", jwtMgr),
			rbacEnforcer.Middleware("lobby", "read"),
		).Get("/check/{code}", lobbyHandler.CheckRoom)
		r.With(
			appMiddleware.EndpointRateLimit(cluster.Ephemeral, "registry:lobbies", jwtMgr),
			rbacEnforcer.Middleware("lobby", "read"),
		).Get("/lobbies", lobbyHandler.ListLobbies)

		r.With(
			appMiddleware.EndpointRateLimit(cluster.Ephemeral, "registry:match", jwtMgr),
			authMiddlewareWrapper(jwtMgr, cluster.Stateful),
			rbacEnforcer.Middleware("lobby", "join"),
		).Post("/match", lobbyHandler.MatchRoom)
	})

	r.Route("/api/v1/lobby", func(r chi.Router) {
		r.With(authMiddlewareWrapper(jwtMgr, cluster.Stateful), rbacEnforcer.Middleware("lobby", "join")).Get("/{code}/ws", lobbyHandler.WebSocket)
	})
}

// setupAdminRoutes registers admin login and admin-protected config routes.
// Rate limiting uses ephemeral Redis (ADR-029).
func setupAdminRoutes(r *chi.Mux, adminHandler *handler.AdminHandler, cluster *store.RedisCluster, jwtMgr *auth.JWTManager, rbacEnforcer *rbac.Enforcer) {
	r.Route("/api/v1/admin", func(r chi.Router) {
		r.With(appMiddleware.EndpointRateLimit(cluster.Ephemeral, "admin:login", jwtMgr)).Post("/login", adminHandler.Login)

		r.Group(func(r chi.Router) {
			r.Use(adminAuthMiddleware(adminHandler))
			r.Post("/logout", adminHandler.Logout)
			r.With(rbacEnforcer.Middleware("config", "read")).Get("/config", adminHandler.GetConfig)
			r.With(rbacEnforcer.Middleware("config", "write")).Patch("/config", adminHandler.UpdateConfig)
			r.With(rbacEnforcer.Middleware("config", "write")).Put("/config", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Deprecation", "true")
				w.Header().Set("Sunset", "Sat, 01 Jan 2027 00:00:00 GMT")
				w.Header().Set("Link", "</api/v1/admin/config>; rel=\"successor-version\"")
				adminHandler.UpdateConfig(w, r)
			})
		})
	})
}

// setupStaticRoutes registers SPA static file serving with path-traversal protection.
func setupStaticRoutes(r *chi.Mux, cfg *handler.Config) {
	if cfg.FrontendDir == "" {
		return
	}
	staticDir := cfg.FrontendDir
	fileServer := http.FileServer(http.Dir(staticDir))

	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		filePath := filepath.Join(staticDir, filepath.Clean(path))

		absPath, err := filepathAbsFn(filePath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		absStaticDir, err := filepathAbsFn(staticDir)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if !strings.HasPrefix(absPath, absStaticDir) {
			http.NotFound(w, r)
			return
		}

		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			if strings.HasSuffix(path, ".html") || !strings.Contains(path, ".") {
				w.Header().Set("Cache-Control", "no-cache")
			} else {
				w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(appConfig.StaticCacheMaxAge))
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		indexPath := filepath.Join(staticDir, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, indexPath)
			return
		}

		http.NotFound(w, r)
	})
}

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
				domain.Unauthorized("Unauthorized").Write(w)
				return
			}
			ctx := domain.WithRole(r.Context(), domain.RoleAdmin)
			ctx = auth.WithJTI(ctx, claims.ID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
