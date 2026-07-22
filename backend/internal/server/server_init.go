package server

import (
	"context"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/bootstrap"
	appConfig "github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/handler"
	appMiddleware "github.com/uppy-clone/backend/internal/middleware"
	"github.com/uppy-clone/backend/internal/rbac"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/worker"
)

// defaultMigrationsDir is the on-disk directory used to locate SQL migration
// files when the environment does not override MigrationsDir.
const defaultMigrationsDir = "migrations"

// initDB connects to PostgreSQL and runs migrations.
func initDB(cfg *handler.Config, timeouts appConfig.TimeoutConfig, deps store.Deps) (*store.PostgresStore, error) {
	db, err := newPostgresStoreFn(cfg.DatabaseURL, timeouts, deps)
	if err != nil {
		slog.Error("failed to connect to PostgreSQL", "error", err)
		return nil, err
	}
	migrationsPath := defaultMigrationsDir
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

// initRedisCluster connects to stateful and ephemeral Redis instances (ADR-029).
// When REDIS_EPHEMERAL_URL is unset, both domains share the stateful instance.
func initRedisCluster(cfg *handler.Config, timeouts appConfig.TimeoutConfig, deps store.Deps) (*store.RedisCluster, error) {
	cluster, err := store.NewRedisCluster(cfg.RedisURL, cfg.RedisEphemeralURL, timeouts, deps)
	if err != nil {
		slog.Error("failed to connect to Redis cluster", "error", err)
		return nil, err
	}
	if cluster.IsSeparated() {
		slog.Info("redis domain separation enabled",
			"stateful", cfg.RedisURL,
			"ephemeral", cfg.RedisEphemeralURL)
	} else {
		slog.Info("redis single-instance mode (set REDIS_EPHEMERAL_URL to enable domain separation)")
	}
	return cluster, nil
}

// initHub creates the Hub and restores rooms from the database.
func initHub(db *store.PostgresStore, redis *store.RedisStore, timeouts appConfig.TimeoutConfig) *game.Hub {
	maxWSConnections := getEnvInt("MAX_WS_CONNECTIONS", appConfig.MaxWSConnections)
	maxPlayersPerRoom := getEnvInt("MAX_PLAYERS_PER_ROOM", appConfig.MaxPlayersPerRoom)
	hub := game.NewHub(db, redis, timeouts, maxWSConnections, maxPlayersPerRoom)
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

// startWorkers starts async workers (EmailWorker, GDPRCleanupWorker).
// All workers receive the Stateful Redis client (ADR-029): email queue is
// stateful data that must survive Redis restarts. Using the ephemeral instance
// here would cause data loss on Redis restart.
func startWorkers(ctx context.Context, wg *sync.WaitGroup, cfg *handler.Config, redis *store.RedisStore, db *store.PostgresStore, timeouts appConfig.TimeoutConfig) {
	// ADR-029: redis is cluster.Stateful — workers must NOT use ephemeral Redis.
	// If you need to pass the cluster instead, use cluster.Stateful.Client() explicitly.
	statefulClient := redis.Client()
	if cfg.ResendAPIKey != "" {
		startWorker(ctx, wg, "email worker", worker.NewEmailWorker(statefulClient, cfg.ResendAPIKey, cfg.EmailFrom, timeouts).Start)
	}

	// ENABLE_EMBEDDED_WORKERS=false (opt-out): GDPR
	// workers are skipped. Default is true (in-process; standalone game-worker
	if !cfg.EnableEmbeddedWorkers {
		slog.Info("embedded workers disabled (ENABLE_EMBEDDED_WORKERS=false, opt-out)")
		return
	}

	retentionDays := getEnvInt("GDPR_RETENTION_DAYS", 30)
	cleanupInterval := time.Duration(getEnvInt("GDPR_CLEANUP_INTERVAL_HOURS", 24)) * time.Hour
	startWorker(ctx, wg, "gdpr cleanup worker", worker.NewGDPRCleanupWorker(db.NewUserRepository(), retentionDays, cleanupInterval).Start)
	slog.Info("gdpr cleanup worker started", "retention_days", retentionDays)
}

// initHandlers creates the auth, lobby, and admin handlers.
func initHandlers(jwtMgr *auth.JWTManager, adminJwtMgr *auth.JWTManager, pg *store.PostgresStore, redis *store.RedisStore, cfg *handler.Config, _ appConfig.TimeoutConfig, hub *game.Hub, deps store.Deps) (*handler.AuthHandler, *handler.LobbyHandler, *handler.AdminHandler, *handler.StatsHandler) {
	var users auth.UserDB
	var configs *store.ConfigRepository
	var results *store.ResultRepository
	if pg != nil {
		users = store.NewUserRepository(pg.Pool(), deps)
		configs = store.NewConfigRepository(pg.Pool(), deps)
		results = store.NewResultRepository(pg.Pool(), deps)
	}

	refreshMgr := auth.NewRefreshTokenManager(redis.Client())
	authHandler := handler.NewAuthHandler(users, redis, jwtMgr, refreshMgr, cfg)
	allowedOrigins := appMiddleware.AllowedOriginsFromEnv(serverEnv.AllowedOrigins)
	lobbyHandler := handler.NewLobbyHandler(hub, allowedOrigins)
	adminHandler := handler.NewAdminHandler(configs, adminJwtMgr, redis)
	statsHandler := handler.NewStatsHandler(results, hub)
	return authHandler, lobbyHandler, adminHandler, statsHandler
}

// initRBAC initializes the RBAC enforcer.
func initRBAC() *rbac.Enforcer {
	return rbac.NewEnforcer()
}

// ─── Server Deps ────────────────────────────────────────────────────

// ServerDeps holds injectable dependencies for the server lifecycle.
// Production code uses DefaultServerDeps(); tests construct custom instances
// to inject mocks without mutating package-level globals.
//
// Shared fields (InitTracer, NewPostgresStore, ShutdownSignals, Exit) live
// in the embedded bootstrap.Deps (spec remediate-structural-debt C3).
// Server-specific fields (ServerShutdown, FilepathAbs) are declared below.
type ServerDeps struct {
	bootstrap.Deps

	// ServerShutdown gracefully shuts down the HTTP server.
	ServerShutdown func(srv *http.Server, ctx context.Context) error

	// FilepathAbs resolves absolute paths (used for static-file path-traversal checks).
	FilepathAbs func(string) (string, error)
}

// DefaultServerDeps returns production-ready dependencies.
func DefaultServerDeps() ServerDeps {
	return ServerDeps{
		Deps:           bootstrap.DefaultDeps(),
		ServerShutdown: bootstrap.HTTPShutdown,
		FilepathAbs:    filepath.Abs,
	}
}
