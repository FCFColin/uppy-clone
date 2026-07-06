package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	goredis "github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker/v2"

	"github.com/uppy-clone/backend/internal/auth"
	appConfig "github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/handler"
	"github.com/uppy-clone/backend/internal/health"
	appMiddleware "github.com/uppy-clone/backend/internal/middleware"
	"github.com/uppy-clone/backend/internal/rbac"
	"github.com/uppy-clone/backend/internal/store"
)

// setupHealthAndMetricsRoutes registers health probes and the metrics endpoint.
func setupHealthAndMetricsRoutes(r *chi.Mux, db *store.PostgresStore, cluster *store.RedisCluster, hub *game.Hub) {

	// ─── Health probes ────────────────────────────────────────────────
	// P2-10: readiness 探测纳入 WebSocket 舱壁负载，连接达上限时返回 503，
	// 避免流量继续打入无法承载的实例。
	var pool *pgxpool.Pool
	if db != nil {
		pool = db.Pool()
	}
	var rdb *goredis.Client
	if cluster != nil {
		rdb = cluster.Stateful.Client()
	}
	healthChecker := health.NewChecker(pool, rdb)
	if hub != nil {
		healthChecker = healthChecker.WithCanAcceptWS(hub.CanAcceptWSConnection)
	}
	r.Get("/health/live", healthChecker.LiveHandler)
	r.Get("/health/ready", healthChecker.ReadyHandler)
	r.Get("/health", healthChecker.ReadyHandler)

	r.Handle("/metrics", metricsAuthMiddleware(promhttp.Handler()))

	// ─── Degradation detection ──────────────────────────────────────────
	var cbs []*gobreaker.CircuitBreaker[any]
	if db != nil {
		cbs = append(cbs, db.CircuitBreaker())
	}
	if cluster != nil {
		cbs = append(cbs, cluster.CircuitBreakers()...)
	}
	r.Get("/health/degraded", handler.DegradedHandler(cbs...))
}

// setupAuthRoutes registers auth and user-data (GDPR) routes.
// Rate limiting uses the ephemeral Redis (ADR-029); auth/session uses stateful Redis.
func setupAuthRoutes(r *chi.Mux, authHandler *handler.AuthHandler, cluster *store.RedisCluster, jwtMgr *auth.JWTManager, rbacEnforcer *rbac.Enforcer) {
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Use(appMiddleware.AuthBulkhead.Middleware)
		r.With(appMiddleware.EndpointRateLimit(cluster.Ephemeral, "auth:quickplay", jwtMgr), appMiddleware.RecordAuthMetrics("quickplay")).Post("/quickplay", authHandler.QuickPlay)
		r.With(appMiddleware.EndpointRateLimit(cluster.Ephemeral, "auth:request", jwtMgr), appMiddleware.RecordAuthMetrics("request")).Post("/request", authHandler.RequestMagicLink)
		r.With(appMiddleware.EndpointRateLimit(cluster.Ephemeral, "auth:verify", jwtMgr), appMiddleware.RecordAuthMetrics("verify")).Get("/verify", authHandler.VerifyMagicLink)
		r.With(appMiddleware.EndpointRateLimit(cluster.Ephemeral, "auth:verify", jwtMgr), appMiddleware.RecordAuthMetrics("verify")).Post("/verify", authHandler.VerifyMagicLinkPost)
		r.With(appMiddleware.RecordAuthMetrics("check")).Get("/check", authHandler.CheckAuth)
		r.With(appMiddleware.RecordAuthMetrics("refresh")).Post("/refresh", authHandler.RefreshToken)
		r.With(appMiddleware.RecordAuthMetrics("logout")).Post("/logout", authHandler.Logout)
	})

	r.Route("/api/v1/user", func(r chi.Router) {
		r.With(authMiddlewareWrapper(jwtMgr, cluster.Stateful), rbacEnforcer.Middleware("user_data", "read")).Get("/data", authHandler.ExportUserData)
		r.With(authMiddlewareWrapper(jwtMgr, cluster.Stateful), rbacEnforcer.Middleware("user_data", "delete")).Delete("/data", authHandler.DeleteUserData)
	})
}

// setupStatsRoutes registers leaderboard and user stats routes.
// Rate limiting uses ephemeral Redis (ADR-029).
func setupStatsRoutes(r *chi.Mux, statsHandler *handler.StatsHandler, cluster *store.RedisCluster, jwtMgr *auth.JWTManager, rbacEnforcer *rbac.Enforcer) {
	if statsHandler == nil {
		return
	}
	r.With(appMiddleware.EndpointRateLimit(cluster.Ephemeral, "stats:leaderboard", jwtMgr)).Get("/api/v1/leaderboard", statsHandler.GetLeaderboard)
	r.With(
		authMiddlewareWrapper(jwtMgr, cluster.Stateful),
		rbacEnforcer.Middleware("user_data", "read"),
	).Get("/api/v1/user/stats", statsHandler.GetUserStats)
}

// setupLobbyRoutes registers registry (room create/check/list) and lobby WebSocket routes.
// Rate limiting + idempotency use ephemeral Redis; auth/session uses stateful Redis (ADR-029).
func setupLobbyRoutes(r *chi.Mux, lobbyHandler *handler.LobbyHandler, cluster *store.RedisCluster, jwtMgr *auth.JWTManager, rbacEnforcer *rbac.Enforcer) {
	r.Route("/api/v1/registry", func(r chi.Router) {
		r.Use(appMiddleware.LobbyBulkhead.Middleware)
		r.With(
			appMiddleware.EndpointRateLimit(cluster.Ephemeral, "registry:create", jwtMgr),
			appMiddleware.IdempotencyMiddleware(cluster.Ephemeral.Client()),
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
		r.Use(appMiddleware.WebSocketBulkhead.Middleware)
		r.With(authMiddlewareWrapper(jwtMgr, cluster.Stateful), rbacEnforcer.Middleware("lobby", "join")).Get("/{code}/ws", lobbyHandler.WebSocket)
	})
}

// filepathAbsFn resolves absolute paths; tests may replace it to simulate errors.
var filepathAbsFn = filepath.Abs

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
