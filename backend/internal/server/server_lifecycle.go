package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/bootstrap"
	appConfig "github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/handler"
	appMiddleware "github.com/uppy-clone/backend/internal/middleware"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/telemetry"
)

func runServer(logger *slog.Logger) error {
	ctx := context.Background()
	shutdown, err := initTracerFn(ctx, "balloon-game", "1.0.0", telemetry.TracerConfig{
		Endpoint:    serverEnv.OTLPEndpoint,
		Insecure:    serverEnv.OTLPInsecure,
		SampleRatio: serverEnv.OTELSampleRatio,
		Environment: serverEnv.Environment,
		Region:      serverEnv.CloudRegion,
	})
	if err != nil {
		return fmt.Errorf("init tracer: %w", err)
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
	initRateLimits()
	if err := initCrypto(cfg); err != nil {
		return fmt.Errorf("init crypto: %w", err)
	}

	deps := bootstrap.NewStoreDeps()

	db, err := initDB(cfg, timeouts, deps)
	if err != nil {
		return err
	}
	defer db.Close()
	audit.InitDBLogger(db.Pool(), serverEnv.AuditSecretOrJWT(), audit.RetryPolicy{
		DBRetry:        store.DefaultDBRetry(),
		MaybeRetryable: store.MaybeRetryable,
	})
	defer audit.CloseDBLogger()

	redis, err := initRedisCluster(cfg, timeouts, deps)
	if err != nil {
		return err
	}
	defer func() { _ = redis.Close() }()

	return serve(ctx, cfg, timeouts, db, redis, deps)
}

func serve(ctx context.Context, cfg *handler.Config, timeouts appConfig.TimeoutConfig, db *store.PostgresStore, cluster *store.RedisCluster, deps store.Deps) error {
	jwtMgr := auth.NewJWTManager(cfg.JWTPrivateKey)
	adminJwtMgr := auth.NewJWTManager(cfg.AdminJWTPrivateKey)

	hub := initHub(db, cluster.Stateful, timeouts)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go hub.CleanupLoop(ctx)
	var wg sync.WaitGroup
	startWorkers(ctx, &wg, cfg, cluster.Stateful, db, timeouts)
	startMetricsCollector(ctx, hub, db, cluster)

	authHandler, lobbyHandler, adminHandler, statsHandler := initHandlers(jwtMgr, adminJwtMgr, db, cluster.Stateful, cfg, timeouts, hub, deps)
	rbacEnforcer := initRBAC()
	r := chi.NewRouter()
	setupRoutes(r, authHandler, lobbyHandler, adminHandler, statsHandler, jwtMgr, db, cluster, rbacEnforcer, cfg, hub)

	srv, serverErr := startServer(r, cfg)
	if err := waitForShutdown(srv, cancel, hub, serverErr); err != nil {
		slog.Error("server error during shutdown", "error", err)
		return err
	}
	wg.Wait()
	return nil
}

// startServer creates and starts the HTTP server in a goroutine.
// Returns the server and a channel that receives an error if ListenAndServe fails.
func startServer(r *chi.Mux, cfg *handler.Config) (*http.Server, <-chan error) {
	addr := ":" + cfg.Port
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  appConfig.ServerReadTimeout,
		WriteTimeout: appConfig.ServerWriteTimeout,
		IdleTimeout:  appConfig.ServerIdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("server starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			errCh <- err
			return
		}
		close(errCh)
	}()

	return srv, errCh
}

// waitForShutdown handles graceful shutdown on SIGINT/SIGTERM.
// Closes all rooms (persisting state) before shutting down the HTTP server (P2-24).
// Returns the server error if ListenAndServe fails, or nil on clean shutdown.
func waitForShutdown(srv *http.Server, cancel context.CancelFunc, hub *game.Hub, serverErr <-chan error) error {
	select {
	case <-shutdownSignals():
		slog.Info("shutting down server...")
	case err := <-serverErr:
		if err != nil {
			return err
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), appConfig.ShutdownTimeout)
	defer shutdownCancel()

	if err := serverShutdownFn(srv, shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}

	hub.CloseAllRooms()

	cancel()
	slog.Info("server stopped")
	return nil
}

// initRateLimits reads rate limit env overrides and applies them.
func initRateLimits() {
	requests := 0
	if v := os.Getenv("RATE_LIMIT_DEFAULT_REQUESTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			requests = n
		}
	}
	window := time.Duration(0)
	if v := os.Getenv("RATE_LIMIT_DEFAULT_WINDOW_SECS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			window = time.Duration(n) * time.Second
		}
	}
	appMiddleware.InitRateLimits(requests, window)
}

// Run is the application entrypoint invoked from cmd/server/main.go.
// On failure, Run calls exitFunc(1) (os.Exit in production) to terminate.
func Run() error {
	logger := initLogger()
	if err := runServer(logger); err != nil {
		logger.Error("server failed", "error", err)
		exitFunc(1)
		return err
	}
	return nil
}
