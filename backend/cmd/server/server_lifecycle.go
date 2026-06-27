package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"

	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/auth"
	appConfig "github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/handler"
	"github.com/uppy-clone/backend/internal/telemetry"
)

func runServer(logger *slog.Logger) {
	ctx := context.Background()
	shutdown, err := telemetry.InitTracer(ctx, "balloon-game", "1.0.0")
	if err != nil {
		slog.Error("failed to initialize tracer", "error", err)
	}
	defer shutdown(ctx)

	initProfiling()

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
	audit.InitDBLogger(db.Pool(), serverEnv.AuditSecretOrJWT())
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
