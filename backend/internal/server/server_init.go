package server

import (
	"context"
	"log/slog"
	"sync"
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

// newPostgresStoreFn is replaceable in unit tests to inject pgxmock-backed stores.
var newPostgresStoreFn = store.NewPostgresStore

// initDB connects to PostgreSQL and runs migrations.
func initDB(cfg *handler.Config, timeouts appConfig.TimeoutConfig) (*store.PostgresStore, error) {
	db, err := newPostgresStoreFn(cfg.DatabaseURL, timeouts)
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

// newRedisStoreFn is replaceable in unit tests.
var newRedisStoreFn = store.NewRedisStore

// initRedis connects to Redis.
func initRedis(cfg *handler.Config, timeouts appConfig.TimeoutConfig) (*store.RedisStore, error) {
	redis, err := newRedisStoreFn(cfg.RedisURL, timeouts)
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

func startWorker(ctx context.Context, wg *sync.WaitGroup, name string, fn func(context.Context)) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		fn(ctx)
		slog.Info(name + " worker stopped")
	}()
	slog.Info(name + " worker started")
}

// startWorkers starts async workers (EmailWorker, GameResultWorker, Outbox Publisher).
// 企业为何需要：异步架构将慢操作（邮件发送、DB批量写入、事件发布）从请求热路径移出，提升响应延迟。
func startWorkers(ctx context.Context, wg *sync.WaitGroup, cfg *handler.Config, redis *store.RedisStore, db *store.PostgresStore, timeouts appConfig.TimeoutConfig) {
	if cfg.ResendAPIKey != "" {
		startWorker(ctx, wg, "email worker", worker.NewEmailWorker(redis.Client(), cfg.ResendAPIKey, cfg.EmailFrom, timeouts).Start)
	}

	startWorker(ctx, wg, "game result worker", worker.NewGameResultWorker(redis.Client(), db.Pool()).Start)
	startWorker(ctx, wg, "outbox publisher", outbox.NewPublisher(db.Pool(), redis.Client()).Start)

	retentionDays := getEnvInt("GDPR_RETENTION_DAYS", 30)
	cleanupInterval := time.Duration(getEnvInt("GDPR_CLEANUP_INTERVAL_HOURS", 24)) * time.Hour
	startWorker(ctx, wg, "gdpr cleanup worker", worker.NewGDPRCleanupWorker(db, retentionDays, cleanupInterval).Start)
	slog.Info("gdpr cleanup worker started", "retention_days", retentionDays)
}

// initHandlers creates the auth, lobby, and admin handlers.
func initHandlers(jwtMgr *auth.JWTManager, adminJwtMgr *auth.JWTManager, pg *store.PostgresStore, redis *store.RedisStore, cfg *handler.Config, timeouts appConfig.TimeoutConfig, hub *game.Hub) (*handler.AuthHandler, *handler.LobbyHandler, *handler.AdminHandler, *handler.StatsHandler) {
	var users handler.UserStore
	var configs handler.ConfigStore
	var results handler.LeaderboardStore
	if pg != nil {
		users = store.NewUserRepository(pg.Pool())
		configs = store.NewConfigRepository(pg.Pool())
		results = store.NewResultRepository(pg.Pool())
	}

	refreshMgr := auth.NewRefreshTokenManager(redis.Client())
	authSvc := newAuthServiceAdapter(jwtMgr, refreshMgr, redis, users, cfg.ResendAPIKey, cfg.EmailFrom, timeouts)
	authHandler := handler.NewAuthHandler(users, redis, authSvc, cfg)
	allowedOrigins := appMiddleware.AllowedOriginsFromEnv(serverEnv.AllowedOrigins)
	lobbyHandler := handler.NewLobbyHandler(hub, allowedOrigins)
	adminHandler := handler.NewAdminHandler(configs, adminJwtMgr, redis)
	statsHandler := handler.NewStatsHandler(results)
	return authHandler, lobbyHandler, adminHandler, statsHandler
}

// initRBAC initializes the RBAC enforcer.
func initRBAC() *rbac.Enforcer {
	return rbac.NewEnforcer()
}
