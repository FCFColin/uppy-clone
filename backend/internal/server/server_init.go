package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/uppy-clone/backend/internal/auth"
	appConfig "github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/handler"
	appMiddleware "github.com/uppy-clone/backend/internal/middleware"
	"github.com/uppy-clone/backend/internal/outbox"
	"github.com/uppy-clone/backend/internal/rbac"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/worker"
)

// initDB connects to PostgreSQL and runs migrations.
func initDB(cfg *handler.Config, timeouts appConfig.TimeoutConfig) (*store.PostgresStore, error) {
	db, err := store.NewPostgresStore(cfg.DatabaseURL, timeouts)
	if err != nil {
		slog.Error("failed to connect to PostgreSQL", "error", err)
		return nil, err
	}
	migrationsPath := "migrations"
	if serverEnv != nil && serverEnv.MigrationsDir != "" {
		migrationsPath = serverEnv.MigrationsDir
	}
	if err := db.RunMigrations(migrationsPath); err != nil {
		if cfg.DatabaseURL != "" {
			slog.Error("migrations failed", "error", err, "path", migrationsPath)
			return nil, err
		}
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

	retentionDays := getEnvInt("GDPR_RETENTION_DAYS", 30)
	cleanupInterval := time.Duration(getEnvInt("GDPR_CLEANUP_INTERVAL_HOURS", 24)) * time.Hour
	gdprWorker := worker.NewGDPRCleanupWorker(db, retentionDays, cleanupInterval)
	go gdprWorker.Start(ctx)
	slog.Info("gdpr cleanup worker started", "retention_days", retentionDays)
}

// initHandlers creates the auth, lobby, and admin handlers.
func initHandlers(jwtMgr *auth.JWTManager, adminJwtMgr *auth.JWTManager, db *store.PostgresStore, redis *store.RedisStore, cfg *handler.Config, timeouts appConfig.TimeoutConfig, hub *game.Hub) (*handler.AuthHandler, *handler.LobbyHandler, *handler.AdminHandler, *handler.StatsHandler) {
	refreshMgr := auth.NewRefreshTokenManager(redis.Client())
	authHandler := handler.NewAuthHandler(jwtMgr, refreshMgr, db, redis, cfg, timeouts)
	allowedOrigins := appMiddleware.AllowedOriginsFromEnv(serverEnv.AllowedOrigins)
	lobbyHandler := handler.NewLobbyHandler(hub, jwtMgr, allowedOrigins)
	adminHandler := handler.NewAdminHandler(db, adminJwtMgr, redis)
	statsHandler := handler.NewStatsHandler(db)
	return authHandler, lobbyHandler, adminHandler, statsHandler
}

// initRBAC initializes the RBAC enforcer.
func initRBAC() *rbac.Enforcer {
	return rbac.NewEnforcer()
}
