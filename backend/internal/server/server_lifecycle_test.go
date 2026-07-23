package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	appConfig "github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/handler"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/telemetry"
	"github.com/uppy-clone/backend/internal/testsecrets"
	"github.com/uppy-clone/backend/internal/testutil"
)

// serverLifecycleMu serializes tests that start runServer/Run (shared audit logger).
var serverLifecycleMu sync.Mutex

func TestInitCrypto(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
	if err := initCrypto(&handler.Config{}); err != nil {
		t.Fatalf("initCrypto: %v", err)
	}
}

func TestInitDB_MigrationFailsNonEmptyURL(t *testing.T) {
	pool := newMockPool(t)
	withMockPostgresStore(t, pool)

	withMigrationsHook(t, func(context.Context, string, string) error {
		return errors.New("migration failed")
	})

	cfg := &handler.Config{DatabaseURL: "postgres://mock/mock?sslmode=disable"}
	_, err := initDB(cfg, appConfig.DefaultTimeoutConfig(), store.DefaultDeps())
	if err == nil {
		t.Fatal("expected migration error when DatabaseURL set")
	}
}

func TestInitDB_SuccessMocked(t *testing.T) {
	pool := newMockPool(t)
	withMockPostgresStore(t, pool)
	withMigrationsHook(t, nil)

	prevEnv := serverEnv
	serverEnv = &appConfig.Env{MigrationsDir: "migrations"}
	t.Cleanup(func() { serverEnv = prevEnv })

	cfg := &handler.Config{DatabaseURL: "postgres://mock/mock?sslmode=disable"}
	db, err := initDB(cfg, appConfig.DefaultTimeoutConfig(), store.DefaultDeps())
	if err != nil {
		t.Fatalf("initDB: %v", err)
	}
	defer db.Close()
}

func TestRunServer_MockDeps(t *testing.T) {
	serverLifecycleMu.Lock()
	t.Cleanup(func() { serverLifecycleMu.Unlock() })

	redisStore := testutil.SetupMiniredisStore(t)
	addr := redisStore.Client().Options().Addr

	pool := newMockPool(t)
	withMockPostgresStore(t, pool)
	withMigrationsHook(t, nil)

	setupRunServerEnv(t, "postgres://mock/mock?sslmode=disable", addr)
	port := bindFreePort(t)
	sigCh := injectShutdownSignal(t)

	done := make(chan error, 1)
	go func() { done <- runServer(slog.Default()) }()

	waitForHealthLive(t, port)

	sigCh <- syscall.SIGTERM
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runServer: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("runServer did not shut down")
	}
}

func TestRunServer_InvalidDatabase(t *testing.T) {
	setupRunServerEnv(t, "postgres://invalid-host:59999/nodb?sslmode=disable&connect_timeout=1", "127.0.0.1:6379")

	err := runServer(slog.Default())
	if err == nil {
		t.Fatal("expected runServer to fail on invalid database")
	}
}

func TestInitRedisCluster_Success(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	addr := redisStore.Client().Options().Addr
	cfg := &handler.Config{RedisURL: addr}
	got, err := initRedisCluster(cfg, appConfig.DefaultTimeoutConfig(), store.DefaultDeps())
	if err != nil {
		t.Fatalf("initRedisCluster: %v", err)
	}
	defer func() { _ = got.Close() }()
}

func TestInitHub_RestoresRooms(t *testing.T) {
	mock := testutil.NewPgxMock(t)

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM lobby_states").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states").
		WithArgs(101).
		WillReturnRows(pgxmock.NewRows([]string{"id", "code", "state", "updated_at", "created_at"}))

	db := store.NewPostgresStoreWithPool(mock)
	redisStore := testutil.SetupMiniredisStore(t)

	serverEnv = &appConfig.Env{MaxWSConnections: 100, MaxPlayersPerRoom: 8}
	t.Cleanup(func() { serverEnv = nil })

	hub := initHub(db, redisStore, appConfig.DefaultTimeoutConfig())
	if hub == nil {
		t.Fatal("initHub returned nil")
	}
}

func TestRunServer_RedisInitFail(t *testing.T) {
	serverLifecycleMu.Lock()
	t.Cleanup(func() { serverLifecycleMu.Unlock() })

	pool := newMockPool(t)
	withMockPostgresStore(t, pool)

	withMigrationsHook(t, nil)

	setupRunServerEnv(t, "postgres://mock/mock?sslmode=disable", "127.0.0.1:59999")

	err := runServer(slog.Default())
	if err == nil {
		t.Fatal("expected runServer to fail on invalid redis")
	}
}

func TestWaitForShutdown_Graceful(t *testing.T) {
	sigCh := injectShutdownSignal(t)

	redisStore := testutil.SetupMiniredisStore(t)
	hub := game.NewHub(nil, redisStore, appConfig.DefaultTimeoutConfig(), 10, 8)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = waitForShutdown(srv.Config, cancel, hub, nil)
		close(done)
	}()

	sigCh <- syscall.SIGTERM

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("waitForShutdown did not complete")
	}

	select {
	case <-ctx.Done():
	default:
		t.Fatal("expected context cancelled after shutdown")
	}
}

func TestServe_StartsAndStops(t *testing.T) {
	serverLifecycleMu.Lock()
	t.Cleanup(func() { serverLifecycleMu.Unlock() })

	sigCh := injectShutdownSignal(t)

	mock := testutil.NewPgxMock(t)
	mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states").
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id", "code", "state", "updated_at", "created_at"}))
	mock.ExpectExec("DELETE FROM users WHERE deleted_at IS NOT NULL").
		WithArgs(pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("DELETE 0"))

	db := store.NewPostgresStoreWithPool(mock)
	redisStore := testutil.SetupMiniredisStore(t)
	cluster := &store.RedisCluster{Stateful: redisStore, Ephemeral: redisStore}

	prevEnv := serverEnv
	serverEnv = &appConfig.Env{
		MaxWSConnections:  100,
		MaxPlayersPerRoom: 8,
		AllowedOrigins:    "http://localhost",
	}
	t.Cleanup(func() { serverEnv = prevEnv })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	cfg := &handler.Config{
		Port:          strconv.Itoa(port),
		JWTPrivateKey: testsecrets.TestJWTPrivateKeyPEM,
		RedisURL:      redisStore.Client().Options().Addr,
	}
	timeouts := appConfig.DefaultTimeoutConfig()
	ctx := context.Background()

	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, cfg, timeouts, db, cluster, store.DefaultDeps())
	}()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://127.0.0.1:" + cfg.Port + "/health/live")
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	sigCh <- syscall.SIGTERM

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not shut down in time")
	}
}

func TestRunServer_TracerInitError(t *testing.T) {
	serverLifecycleMu.Lock()
	t.Cleanup(func() { serverLifecycleMu.Unlock() })

	prevTracer := initTracerFn
	initTracerFn = func(context.Context, string, string, telemetry.TracerConfig) (func(context.Context) error, error) {
		return nil, errors.New("tracer init failed")
	}
	t.Cleanup(func() { initTracerFn = prevTracer })

	redisStore := testutil.SetupMiniredisStore(t)
	pool := newMockPool(t)
	withMockPostgresStore(t, pool)
	withMigrationsHook(t, nil)

	setupRunServerEnv(t, "postgres://mock/mock?sslmode=disable", redisStore.Client().Options().Addr)

	done := make(chan error, 1)
	go func() { done <- runServer(slog.Default()) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected tracer init error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runServer did not fail")
	}
}

// setupRunServerEnv sets the env vars + serverEnv required by runServer-based
// tests. The DatabaseURL and RedisURL are taken from the caller (mock or real).
// serverEnv is restored on t.Cleanup.
func setupRunServerEnv(t *testing.T, dbURL, redisAddr string) {
	t.Helper()
	t.Setenv("ENABLE_HSTS", "false")
	t.Setenv("JWT_PRIVATE_KEY", testsecrets.TestJWTPrivateKeyPEM)
	t.Setenv("DATABASE_URL", dbURL)
	t.Setenv("REDIS_URL", redisAddr)
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)

	prevEnv := serverEnv
	serverEnv = appConfig.Load()
	serverEnv.EnableHSTS = false
	serverEnv.MigrationsDir = "migrations"
	serverEnv.AllowedOrigins = "http://localhost"
	t.Cleanup(func() { serverEnv = prevEnv })
}

// bindFreePort finds a free TCP port, sets PORT=<port>, and returns the port.
func bindFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	t.Setenv("PORT", strconv.Itoa(port))
	return port
}

// injectShutdownSignal replaces shutdownSignals with a channel the test controls.
// Returns the channel and restores the original on t.Cleanup.
func injectShutdownSignal(t *testing.T) chan os.Signal {
	t.Helper()
	sigCh := make(chan os.Signal, 1)
	prev := shutdownSignals
	shutdownSignals = func() <-chan os.Signal { return sigCh }
	t.Cleanup(func() { shutdownSignals = prev })
	return sigCh
}

// waitForHealthLive polls /health/live on 127.0.0.1:port until it responds or
// the deadline (5s) elapses. Failures are non-fatal — callers assert via the
// outer runServer result.
func waitForHealthLive(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/health/live")
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}


