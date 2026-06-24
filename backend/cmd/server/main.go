package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/auth"
	appConfig "github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/handler"
	"github.com/uppy-clone/backend/internal/health"
	"github.com/uppy-clone/backend/internal/metrics"
	appMiddleware "github.com/uppy-clone/backend/internal/middleware"
	"github.com/uppy-clone/backend/internal/outbox"
	"github.com/uppy-clone/backend/internal/rbac"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/telemetry"
	"github.com/uppy-clone/backend/internal/worker"
)

func main() {
	logger := initLogger()

	ctx := context.Background()
	shutdown, err := telemetry.InitTracer(ctx, "balloon-game", "1.0.0")
	if err != nil {
		slog.Error("failed to initialize tracer", "error", err)
	}
	defer shutdown(ctx)

	// Continuous profiling with Pyroscope (optional)
	// 企业为何需要：always-on profiling 让团队随时查看 CPU/内存火焰图，无需手动抓取。
	if os.Getenv("ENABLE_PYROSCOPE") == "true" {
		pyroscopeAddress := os.Getenv("PYROSCOPE_SERVER_ADDRESS")
		if pyroscopeAddress != "" {
			// TODO: add github.com/grafana/pyroscope-go dependency to enable always-on profiling.
			// Note: requires github.com/grafana/pyroscope-go dependency
			// pyroscope.Start(pyroscope.Config{
			// 	ApplicationName: "balloon-game",
			// 	ServerAddress:   pyroscopeAddress,
			// 	Logger:          slog.NewLogLogger(logger.Handler(), slog.LevelInfo),
			// 	ProfileTypes: []pyroscope.ProfileType{
			// 		pyroscope.ProfileCPU,
			// 		pyroscope.ProfileAllocObjects,
			// 		pyroscope.ProfileAllocSpace,
			// 		pyroscope.ProfileInuseObjects,
			// 		pyroscope.ProfileInuseSpace,
			// 	},
			// })
			slog.Info("Pyroscope continuous profiling enabled", "address", pyroscopeAddress)
		}
	}

	timeouts := appConfig.DefaultTimeoutConfig()
	cfg := loadConfig()
	validateConfig(cfg, logger)
	if err := initCrypto(cfg); err != nil {
		logger.Error("ENCRYPTION_KEY environment variable is required", "error", err)
		shutdown(ctx)
		os.Exit(1)
	}

	db, err := initDB(cfg, timeouts)
	if err != nil {
		os.Exit(1)
	}
	defer db.Close()
	audit.InitDBLogger(db.Pool(), getEnv("AUDIT_SECRET", cfg.JWTSecret))
	defer audit.CloseDBLogger()

	redis, err := initRedis(cfg, timeouts)
	if err != nil {
		os.Exit(1)
	}
	defer func() { _ = redis.Close() }()

	jwtMgr := auth.NewJWTManager(cfg.JWTSecret)
	broadcaster := game.NewPubSubBroadcaster(redis.Client())
	hub := initHub(db, redis, timeouts, broadcaster)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go hub.CleanupLoop(ctx)
	startWorkers(ctx, cfg, redis, db, timeouts)
	startMetricsCollector(ctx, hub, db, redis)

	authHandler, lobbyHandler, adminHandler := initHandlers(jwtMgr, db, redis, cfg, timeouts, hub)
	rbacEnforcer := initRBAC()
	r := chi.NewRouter()
	setupRoutes(r, authHandler, lobbyHandler, adminHandler, jwtMgr, db, redis, rbacEnforcer, cfg, hub)

	srv := startServer(r, cfg)
	waitForShutdown(srv, cancel, hub, broadcaster)
}

// initLogger sets up the structured logger.
// Enterprise rationale: Structured JSON logs are required for log aggregation
// systems (ELK, Loki, Datadog). Text logs cannot be efficiently queried.
// LOG_FORMAT=text switches to text format for local development DX.
func initLogger() *slog.Logger {
	logLevel := parseLogLevel(getEnv("LOG_LEVEL", "info"))
	opts := &slog.HandlerOptions{Level: logLevel}
	var handler slog.Handler
	if getEnv("LOG_FORMAT", "json") == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

// loadConfig loads configuration from environment variables.
func loadConfig() *handler.Config {
	return &handler.Config{
		ResendAPIKey:  getEnv("RESEND_API_KEY", ""),
		EmailFrom:     getEnv("EMAIL_FROM", ""),
		AdminPassword: getEnv("ADMIN_PASSWORD", ""),
		JWTSecret:     getEnv("JWT_SECRET", ""),
		DatabaseURL:   getEnv("DATABASE_URL", ""),
		RedisURL:      getEnv("REDIS_URL", "localhost:6379"),
		Port:          getEnv("PORT", "8080"),
		FrontendDir:   getEnv("FRONTEND_DIR", ""),
	}
}

// validateConfig validates required config fields and rejects weak dev secrets in production.
func validateConfig(cfg *handler.Config, logger *slog.Logger) {
	if cfg.JWTSecret == "" {
		logger.Error("JWT_SECRET environment variable is required")
		os.Exit(1)
	}
	// Weak-key detection: reject known dev/default secrets in production mode.
	// ENABLE_HSTS defaults to enabled (production); only "false" opts out (dev).
	if (strings.Contains(cfg.JWTSecret, "DEV_ONLY") || strings.Contains(cfg.JWTSecret, "change-in-production")) && os.Getenv("ENABLE_HSTS") != "false" {
		logger.Error("JWT_SECRET contains a known weak/dev value; refuse to start in production mode (set ENABLE_HSTS=false only for local dev)")
		os.Exit(1)
	}
	if cfg.DatabaseURL == "" {
		logger.Error("DATABASE_URL environment variable is required")
		os.Exit(1)
	}
}

// initCrypto initializes the AES encryption key from the environment.
// 企业为何需要：全零密钥让 AES-256-GCM 加密形同虚设。
// 生产环境必须配置独立密钥，未配置时 fail-fast 防止静默降级为明文。
func initCrypto(_ *handler.Config) error {
	return crypto.InitFromEnv()
}

// initDB connects to PostgreSQL and runs migrations.
func initDB(cfg *handler.Config, timeouts appConfig.TimeoutConfig) (*store.PostgresStore, error) {
	db, err := store.NewPostgresStore(cfg.DatabaseURL, timeouts)
	if err != nil {
		slog.Error("failed to connect to PostgreSQL", "error", err)
		return nil, err
	}
	migrationsPath := "migrations"
	if err := db.RunMigrations(migrationsPath); err != nil {
		slog.Warn("migrations warning", "error", err)
	}
	return db, nil
}

// initRedis connects to Redis.
func initRedis(cfg *handler.Config, timeouts appConfig.TimeoutConfig) (*store.RedisStore, error) {
	redis, err := store.NewRedisStore(cfg.RedisURL, timeouts)
	if err != nil {
		slog.Error("failed to connect to Redis", "error", err)
		return nil, err
	}
	return redis, nil
}

// initHub creates the Hub and restores rooms from the database.
// 企业为何需要：舱壁隔离（Bulkhead）防止单类资源耗尽拖垮整体。
func initHub(db *store.PostgresStore, redis *store.RedisStore, timeouts appConfig.TimeoutConfig, broadcaster *game.PubSubBroadcaster) *game.Hub {
	maxWSConnections := getEnvInt("MAX_WS_CONNECTIONS", appConfig.MaxWSConnections)
	maxPlayersPerRoom := getEnvInt("MAX_PLAYERS_PER_ROOM", appConfig.MaxPlayersPerRoom)
	hub := game.NewHub(db, redis, timeouts, maxWSConnections, maxPlayersPerRoom, broadcaster)
	if err := hub.RestoreRooms(); err != nil {
		slog.Warn("failed to restore rooms", "error", err)
	}
	return hub
}

// startWorkers starts async workers (EmailWorker, GameResultWorker, Outbox Publisher).
// 企业为何需要：异步架构将慢操作（邮件发送、DB批量写入、事件发布）从请求热路径移出，提升响应延迟。
func startWorkers(ctx context.Context, cfg *handler.Config, redis *store.RedisStore, db *store.PostgresStore, timeouts appConfig.TimeoutConfig) {
	if cfg.ResendAPIKey != "" {
		emailWorker := worker.NewEmailWorker(redis.Client(), cfg.ResendAPIKey, cfg.EmailFrom, timeouts)
		go emailWorker.Start(ctx)
		slog.Info("email worker started")
	}

	gameResultWorker := worker.NewGameResultWorker(redis.Client(), db.Pool())
	go gameResultWorker.Start(ctx)
	slog.Info("game result worker started")

	outboxPublisher := outbox.NewPublisher(db.Pool(), redis.Client())
	go outboxPublisher.Start(ctx)
	slog.Info("outbox publisher started")

	// TODO: add scheduled cleanup goroutine to hard-delete users with deleted_at older than 30 days
}

// startMetricsCollector starts all 3 Prometheus metrics goroutines.
func startMetricsCollector(ctx context.Context, hub *game.Hub, db *store.PostgresStore, redis *store.RedisStore) {
	// Periodically update business metrics for Prometheus
	go func() {
		ticker := time.NewTicker(appConfig.MetricsInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				metrics.ActiveRooms.Set(float64(hub.RoomCount()))
				metrics.ActivePlayers.Set(float64(hub.PlayerCount()))

				// Monitor stream length for consumer lag
				if streamLen, err := redis.Client().XLen(ctx, "game:results").Result(); err == nil {
					metrics.GameResultsStreamLen.Set(float64(streamLen))
				}
				if emailLen, err := redis.Client().XLen(ctx, "email:queue").Result(); err == nil {
					metrics.EmailQueueStreamLen.Set(float64(emailLen))
				}
			}
		}
	}()

	// Periodically update DB pool metrics for Prometheus
	// Includes DBPoolAcquireDuration observation via delta sampling.
	go func() {
		ticker := time.NewTicker(appConfig.MetricsInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				db.ObservePoolStats()
			}
		}
	}()

	// Periodically update Redis pool metrics for Prometheus
	go func() {
		ticker := time.NewTicker(appConfig.MetricsInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stats := redis.PoolStats()
				metrics.RedisPoolIdleConns.Set(float64(stats.IdleConns))
				metrics.RedisPoolTotalConns.Set(float64(stats.TotalConns))
			}
		}
	}()
}

// initHandlers creates the auth, lobby, and admin handlers.
func initHandlers(jwtMgr *auth.JWTManager, db *store.PostgresStore, redis *store.RedisStore, cfg *handler.Config, timeouts appConfig.TimeoutConfig, hub *game.Hub) (*handler.AuthHandler, *handler.LobbyHandler, *handler.AdminHandler) {
	refreshMgr := auth.NewRefreshTokenManager(redis.Client())
	authHandler := handler.NewAuthHandler(jwtMgr, refreshMgr, db, redis, cfg, timeouts)
	allowedOrigins := appMiddleware.AllowedOriginsFromEnv(getEnv("ALLOWED_ORIGINS", ""))
	lobbyHandler := handler.NewLobbyHandler(hub, jwtMgr, allowedOrigins)
	adminHandler := handler.NewAdminHandler(db, jwtMgr, redis)
	return authHandler, lobbyHandler, adminHandler
}

// initRBAC initializes the RBAC enforcer.
func initRBAC() *rbac.Enforcer {
	rbacEnforcer, err := rbac.NewEnforcer("internal/rbac/model.conf", "internal/rbac/policy.csv")
	if err != nil {
		slog.Error("failed to initialize RBAC", "error", err)
	}
	return rbacEnforcer
}

// setupRoutes registers all HTTP routes and middleware on the chi router.
func setupRoutes(r *chi.Mux, authHandler *handler.AuthHandler, lobbyHandler *handler.LobbyHandler, adminHandler *handler.AdminHandler, jwtMgr *auth.JWTManager, db *store.PostgresStore, redis *store.RedisStore, rbacEnforcer *rbac.Enforcer, cfg *handler.Config, hub *game.Hub) {
	setupMiddleware(r)
	setupHealthAndMetricsRoutes(r, db, redis, hub)
	setupAuthRoutes(r, authHandler, redis, jwtMgr, rbacEnforcer)
	setupLobbyRoutes(r, lobbyHandler, redis, jwtMgr, rbacEnforcer)
	setupAdminRoutes(r, adminHandler, redis, jwtMgr, rbacEnforcer)
	setupStaticRoutes(r, cfg)
}

// setupMiddleware registers global chi middleware (logging, recovery, security, CORS).
func setupMiddleware(r *chi.Mux) {
	r.Use(chiMiddleware.Logger)
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.RequestID)
	r.Use(appMiddleware.RequestIDLogger)
	r.Use(appMiddleware.TracingMiddleware)
	r.Use(appMiddleware.PrometheusMiddleware)
	r.Use(appMiddleware.SecurityHeaders)

	// CORS
	allowedOrigins := appMiddleware.AllowedOriginsFromEnv(getEnv("ALLOWED_ORIGINS", ""))
	r.Use(appMiddleware.CORS(allowedOrigins))
}

// setupHealthAndMetricsRoutes registers health probes and the metrics endpoint.
func setupHealthAndMetricsRoutes(r *chi.Mux, db *store.PostgresStore, redis *store.RedisStore, hub *game.Hub) {

	// ─── Health probes ────────────────────────────────────────────────
	// P2-10: readiness 探测纳入 WebSocket 舱壁负载，连接达上限时返回 503，
	// 避免流量继续打入无法承载的实例。
	healthChecker := health.NewChecker(db.Pool(), redis.Client()).WithCanAcceptWS(hub.CanAcceptWSConnection)
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

		r.With(rbacEnforcer.Middleware("lobby", "read")).Get("/check/{code}", lobbyHandler.CheckRoom)
		r.With(rbacEnforcer.Middleware("lobby", "read")).Get("/lobbies", lobbyHandler.ListLobbies)
	})

	r.Route("/api/v1/lobby", func(r chi.Router) {
		r.Use(appMiddleware.WebSocketBulkhead.Middleware)
		r.With(authMiddlewareWrapper(jwtMgr, redis), rbacEnforcer.Middleware("lobby", "join")).Get("/{code}/ws", lobbyHandler.WebSocket)
	})
}

// setupAdminRoutes registers admin login and admin-protected config routes.
func setupAdminRoutes(r *chi.Mux, adminHandler *handler.AdminHandler, redis *store.RedisStore, jwtMgr *auth.JWTManager, rbacEnforcer *rbac.Enforcer) {
	r.Route("/api/v1/admin", func(r chi.Router) {
		r.Use(appMiddleware.AdminBulkhead.Middleware)
		r.With(appMiddleware.EndpointRateLimit(redis, "admin:login", jwtMgr)).Post("/login", adminHandler.Login)

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

// metricsAuthMiddleware wraps a handler with Basic Auth for /metrics endpoint.
// 企业为何需要：/metrics 暴露运行时指标（GC、内存、连接池），攻击者可利用这些信息进行精准攻击。
func metricsAuthMiddleware(next http.Handler) http.Handler {
	user := os.Getenv("METRICS_USER")
	pass := os.Getenv("METRICS_PASSWORD")
	// If no credentials configured, allow access (dev mode)
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

// startServer creates and starts the HTTP server in a goroutine.
func startServer(r *chi.Mux, cfg *handler.Config) *http.Server {
	addr := ":" + cfg.Port
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  appConfig.ServerReadTimeout,
		WriteTimeout: appConfig.ServerWriteTimeout,
		IdleTimeout:  appConfig.ServerIdleTimeout,
	}

	go func() {
		slog.Info("server starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	return srv
}

// waitForShutdown handles graceful shutdown on SIGINT/SIGTERM.
// Closes all rooms (persisting state) before shutting down the HTTP server (P2-24).
func waitForShutdown(srv *http.Server, cancel context.CancelFunc, hub *game.Hub, broadcaster *game.PubSubBroadcaster) {
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	<-done
	slog.Info("shutting down server...")

	// Close all rooms and persist state before shutting down (P2-24).
	hub.CloseAllRooms()

	// 关闭跨实例广播订阅（ADR-005）
	if broadcaster != nil {
		if err := broadcaster.Close(); err != nil {
			slog.Error("broadcaster close error", "error", err)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), appConfig.ShutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}

	cancel() // stop hub cleanup loop
	slog.Info("server stopped")
}

// getEnv returns the environment variable value or a default.
func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// getEnvInt returns the environment variable value as int, or a default.
func getEnvInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		slog.Warn("invalid env var, using default", "key", key, "value", val, "default", defaultVal)
		return defaultVal
	}
	return n
}

// parseLogLevel converts a log level string to slog.Level.
func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
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
