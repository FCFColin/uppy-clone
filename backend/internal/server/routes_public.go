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
func setupHealthAndMetricsRoutes(r *chi.Mux, db *store.PostgresStore, redis *store.RedisStore, hub *game.Hub) {

	// ─── Health probes ────────────────────────────────────────────────
	// P2-10: readiness 探测纳入 WebSocket 舱壁负载，连接达上限时返回 503，
	// 避免流量继续打入无法承载的实例。
	var pool *pgxpool.Pool
	if db != nil {
		pool = db.Pool()
	}
	var rdb *goredis.Client
	if redis != nil {
		rdb = redis.Client()
	}
	healthChecker := health.NewChecker(pool, rdb)
	if hub != nil {
		healthChecker = healthChecker.WithCanAcceptWS(hub.CanAcceptWSConnection)
	}
	r.Get("/health/live", healthChecker.LiveHandler)
	r.Get("/health/ready", healthChecker.ReadyHandler)
	r.Get("/health", healthChecker.ReadyHandler)

	r.Handle("/metrics", metricsAuthMiddleware(promhttp.Handler()))
}

// setupAuthRoutes registers auth and user-data (GDPR) routes.
func setupAuthRoutes(r *chi.Mux, authHandler *handler.AuthHandler, redis *store.RedisStore, jwtMgr *auth.JWTManager, rbacEnforcer *rbac.Enforcer) {
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Use(appMiddleware.AuthBulkhead.Middleware)
		r.With(appMiddleware.EndpointRateLimit(redis, "auth:quickplay", jwtMgr)).Post("/quickplay", authHandler.QuickPlay)
		r.With(appMiddleware.EndpointRateLimit(redis, "auth:request", jwtMgr)).Post("/request", authHandler.RequestMagicLink)
		r.With(appMiddleware.EndpointRateLimit(redis, "auth:verify", jwtMgr)).Get("/verify", authHandler.VerifyMagicLink)
		r.Get("/check", authHandler.CheckAuth)
		r.Post("/refresh", authHandler.RefreshToken)
		r.Post("/logout", authHandler.Logout)
	})

	r.Route("/api/v1/user", func(r chi.Router) {
		r.With(authMiddlewareWrapper(jwtMgr, redis), rbacEnforcer.Middleware("user_data", "read")).Get("/data", authHandler.ExportUserData)
		r.With(authMiddlewareWrapper(jwtMgr, redis), rbacEnforcer.Middleware("user_data", "delete")).Delete("/data", authHandler.DeleteUserData)
	})
}

// setupStatsRoutes registers leaderboard and user stats routes.
func setupStatsRoutes(r *chi.Mux, statsHandler *handler.StatsHandler, redis *store.RedisStore, jwtMgr *auth.JWTManager, rbacEnforcer *rbac.Enforcer) {
	if statsHandler == nil {
		return
	}
	r.With(appMiddleware.EndpointRateLimit(redis, "stats:leaderboard", jwtMgr)).Get("/api/v1/leaderboard", statsHandler.GetLeaderboard)
	r.With(
		authMiddlewareWrapper(jwtMgr, redis),
		rbacEnforcer.Middleware("user_data", "read"),
	).Get("/api/v1/user/stats", statsHandler.GetUserStats)
}

// setupLobbyRoutes registers registry (room create/check/list) and lobby WebSocket routes.
func setupLobbyRoutes(r *chi.Mux, lobbyHandler *handler.LobbyHandler, redis *store.RedisStore, jwtMgr *auth.JWTManager, rbacEnforcer *rbac.Enforcer) {
	r.Route("/api/v1/registry", func(r chi.Router) {
		r.Use(appMiddleware.LobbyBulkhead.Middleware)
		r.With(
			appMiddleware.EndpointRateLimit(redis, "registry:create", jwtMgr),
			appMiddleware.IdempotencyMiddleware(redis.Client()),
			authMiddlewareWrapper(jwtMgr, redis),
			rbacEnforcer.Middleware("lobby", "create"),
		).Post("/create", lobbyHandler.CreateRoom)

		r.With(
			appMiddleware.EndpointRateLimit(redis, "registry:check", jwtMgr),
			rbacEnforcer.Middleware("lobby", "read"),
		).Get("/check/{code}", lobbyHandler.CheckRoom)
		r.With(
			appMiddleware.EndpointRateLimit(redis, "registry:lobbies", jwtMgr),
			rbacEnforcer.Middleware("lobby", "read"),
		).Get("/lobbies", lobbyHandler.ListLobbies)

		r.With(
			appMiddleware.EndpointRateLimit(redis, "registry:match", jwtMgr),
			authMiddlewareWrapper(jwtMgr, redis),
			rbacEnforcer.Middleware("lobby", "join"),
		).Post("/match", lobbyHandler.MatchRoom)
	})

	r.Route("/api/v1/lobby", func(r chi.Router) {
		r.Use(appMiddleware.WebSocketBulkhead.Middleware)
		r.With(authMiddlewareWrapper(jwtMgr, redis), rbacEnforcer.Middleware("lobby", "join")).Get("/{code}/ws", lobbyHandler.WebSocket)
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

		absPath, err := filepath.Abs(filePath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		absStaticDir, err := filepath.Abs(staticDir)
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
