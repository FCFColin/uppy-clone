package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/grafana/pyroscope-go"

	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/bootstrap"
	appConfig "github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/handler"
	"github.com/uppy-clone/backend/internal/metrics"
	appMiddleware "github.com/uppy-clone/backend/internal/middleware"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/telemetry"
)

func runServer(logger *slog.Logger) error {
	ctx := context.Background()

	timeouts := appConfig.DefaultTimeoutConfig()
	cfg := loadConfig()
	validateConfig(cfg, logger)

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

type pyroscopeLogger struct{}

func (pyroscopeLogger) Infof(msg string, args ...interface{}) {
	slog.Info(fmt.Sprintf(msg, args...))
}

func (pyroscopeLogger) Debugf(msg string, args ...interface{}) {
	slog.Debug(fmt.Sprintf(msg, args...))
}

func (pyroscopeLogger) Errorf(msg string, args ...interface{}) {
	slog.Error(fmt.Sprintf(msg, args...))
}

func initProfiling() {
	if os.Getenv("ENABLE_PYROSCOPE") != "true" {
		return
	}
	address := os.Getenv("PYROSCOPE_SERVER_ADDRESS")
	if address == "" {
		slog.Warn("PYROSCOPE_SERVER_ADDRESS not set, skipping profiling")
		return
	}

	_, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: "balloon-game",
		ServerAddress:   address,
		Logger:          pyroscopeLogger{},
		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileInuseSpace,
		},
	})
	if err != nil {
		slog.Error("failed to start pyroscope", "error", err)
		return
	}
	slog.Info("Pyroscope continuous profiling enabled", "address", address)
}

func initLogger() *slog.Logger {
	logLevel := parseLogLevel(getEnv("LOG_LEVEL", "info"))
	opts := &slog.HandlerOptions{Level: logLevel}

	// Build writer: stdout + optional log file.
	var writer io.Writer = os.Stdout
	logFile := getEnv("LOG_FILE", "")
	if logFile == "" {
		// Default: write to temp dir next to the backend dir.
		if dir, err := os.Getwd(); err == nil {
			logFile = filepath.Join(dir, "server.log")
		}
	}
	if logFile != "" {
		//#nosec G304,G302 -- logFile comes from trusted env var (LOG_FILE) or is derived from os.Getwd(); 0644 is intentional for log files readable by ops tooling.
		if f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
			writer = io.MultiWriter(os.Stdout, f)
		} else {
			slog.Warn("failed to open log file, logging to stdout only", "path", logFile, "error", err)
		}
	}

	var handler slog.Handler
	if getEnv("LOG_FORMAT", "json") == "text" {
		handler = slog.NewTextHandler(writer, opts)
	} else {
		handler = slog.NewJSONHandler(writer, opts)
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

// metricsCollectInterval controls ticker cadence; tests may shorten it for -short runs.
var metricsCollectInterval = appConfig.MetricsInterval

// startMetricsCollector starts all 3 Prometheus metrics goroutines.
func startMetricsCollector(ctx context.Context, hub *game.Hub, db *store.PostgresStore, cluster *store.RedisCluster) {
	runCollector(ctx, func() {
		metrics.ActiveRooms.Set(float64(hub.RoomCount()))
		metrics.ActivePlayers.Set(float64(hub.PlayerCount()))
		phaseCounts := hub.PhaseCounts()
		for _, phase := range []string{"waiting", "countdown", "playing", "ended"} {
			metrics.RoomsByPhase.WithLabelValues(phase).Set(float64(phaseCounts[phase]))
		}
		if emailLen, err := cluster.Stateful.Client().XLen(ctx, "email:queue").Result(); err == nil {
			metrics.EmailQueueStreamLen.Set(float64(emailLen))
		}
	})

	runCollector(ctx, func() {
		db.ObservePoolStats()
	})

	runCollector(ctx, func() {
		stats := cluster.Stateful.PoolStats()
		metrics.RedisPoolIdleConns.Set(float64(stats.IdleConns))
		metrics.RedisPoolTotalConns.Set(float64(stats.TotalConns))
	})
}

func runCollector(ctx context.Context, fn func()) {
	go func() {
		ticker := time.NewTicker(metricsCollectInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				fn()
			}
		}
	}()
}
