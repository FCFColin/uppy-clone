package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/go-chi/chi/v5"

	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/auth"
	appConfig "github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/handler"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/telemetry"
)

// shutdownSignals returns the OS signal channel used for graceful shutdown.
// Tests may replace this to inject signals without sending real SIGTERM.
var shutdownSignals = func() <-chan os.Signal {
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)
	return done
}

// initTracerFn is replaceable in unit tests.
var initTracerFn = telemetry.InitTracer

// serverShutdownFn is replaceable in unit tests (http.Server.Shutdown).
var serverShutdownFn = func(srv *http.Server, ctx context.Context) error {
	return srv.Shutdown(ctx)
}

func runServer(logger *slog.Logger) error {
	ctx := context.Background()
	shutdown, err := initTracerFn(ctx, "balloon-game", "1.0.0")
	if err != nil {
		slog.Error("failed to initialize tracer", "error", err)
	}
	stopTracer := func() {
		if shutdown != nil {
			if err := shutdown(ctx); err != nil {
				slog.Warn("tracer shutdown", "error", err)
			}
		}
	}
	defer stopTracer()

	initProfiling()

	timeouts := appConfig.DefaultTimeoutConfig()
	cfg := loadConfig()
	validateConfig(cfg, logger)
	if err := initCrypto(cfg); err != nil {
		return fmt.Errorf("init crypto: %w", err)
	}

	db, err := initDB(cfg, timeouts)
	if err != nil {
		return err
	}
	defer db.Close()
	audit.InitDBLogger(db.Pool(), serverEnv.AuditSecretOrJWT())
	defer audit.CloseDBLogger()

	redis, err := initRedis(cfg, timeouts)
	if err != nil {
		return err
	}
	defer func() { _ = redis.Close() }()

	return serve(ctx, cfg, timeouts, db, redis)
}

func serve(ctx context.Context, cfg *handler.Config, timeouts appConfig.TimeoutConfig, db *store.PostgresStore, redis *store.RedisStore) error {
	jwtMgr := auth.NewJWTManager(cfg.JWTPrivateKey)
	adminJwtMgr := auth.NewJWTManager(cfg.JWTPrivateKey)
	broadcaster := game.NewPubSubBroadcaster(redis.Client())
	hub := initHub(db, redis, timeouts, broadcaster)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go hub.CleanupLoop(ctx)
	var wg sync.WaitGroup
	startWorkers(ctx, &wg, cfg, redis, db, timeouts)
	startMetricsCollector(ctx, hub, db, redis)

	authHandler, lobbyHandler, adminHandler, statsHandler := initHandlers(jwtMgr, adminJwtMgr, db, redis, cfg, timeouts, hub)
	rbacEnforcer := initRBAC()
	r := chi.NewRouter()
	setupRoutes(r, authHandler, lobbyHandler, adminHandler, statsHandler, jwtMgr, db, redis, rbacEnforcer, cfg, hub)

	srv := startServer(r, cfg)
	waitForShutdown(srv, cancel, hub, broadcaster)
	wg.Wait()
	return nil
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
	<-shutdownSignals()
	slog.Info("shutting down server...")

	hub.CloseAllRooms()

	if broadcaster != nil {
		if err := broadcaster.Close(); err != nil {
			slog.Error("broadcaster close error", "error", err)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), appConfig.ShutdownTimeout)
	defer shutdownCancel()

	if err := serverShutdownFn(srv, shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}

	cancel()
	slog.Info("server stopped")
}

// exitFunc is replaceable in unit tests (Run calls os.Exit on failure).
var exitFunc = os.Exit

// Run is the application entrypoint invoked from cmd/server/main.go.
func Run() {
	logger := initLogger()
	if err := runServer(logger); err != nil {
		logger.Error("server failed", "error", err)
		exitFunc(1)
	}
}
