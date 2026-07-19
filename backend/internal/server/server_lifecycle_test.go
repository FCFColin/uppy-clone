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

	"github.com/jackc/pgx/v5"
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

func tryPostgresURL(t *testing.T) string {
	t.Helper()
	connStr := os.Getenv("TEST_DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://test:test@127.0.0.1:5432/testdb?sslmode=disable&connect_timeout=2"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	_ = conn.Close(ctx)
	return connStr
}

func TestInitCrypto(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
	if err := initCrypto(&handler.Config{}); err != nil {
		t.Fatalf("initCrypto: %v", err)
	}
}

func TestInitLogger(t *testing.T) {
	t.Setenv("LOG_FORMAT", "text")
	t.Setenv("LOG_LEVEL", "debug")
	if logger := initLogger(); logger == nil {
		t.Fatal("initLogger returned nil")
	}
	t.Setenv("LOG_FORMAT", "json")
	if logger := initLogger(); logger == nil {
		t.Fatal("initLogger json returned nil")
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
	withMockRedisStore(t, redisStore)
	withMigrationsHook(t, nil)

	t.Setenv("ENABLE_HSTS", "false")
	t.Setenv("JWT_PRIVATE_KEY", testsecrets.TestJWTPrivateKeyPEM)
	t.Setenv("DATABASE_URL", "postgres://mock/mock?sslmode=disable")
	t.Setenv("REDIS_URL", addr)
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)

	prevEnv := serverEnv
	serverEnv = appConfig.Load()
	serverEnv.EnableHSTS = false
	serverEnv.MigrationsDir = "migrations"
	serverEnv.AllowedOrigins = "http://localhost"
	t.Cleanup(func() { serverEnv = prevEnv })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	t.Setenv("PORT", strconv.Itoa(port))

	sigCh := make(chan os.Signal, 1)
	prev := shutdownSignals
	shutdownSignals = func() <-chan os.Signal { return sigCh }
	t.Cleanup(func() { shutdownSignals = prev })

	done := make(chan error, 1)
	go func() { done <- runServer(slog.Default()) }()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/health/live")
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

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

func TestRun_SuccessMocked(t *testing.T) {
	serverLifecycleMu.Lock()
	t.Cleanup(func() { serverLifecycleMu.Unlock() })

	origExit := exitFunc
	exitFunc = func(code int) {
		t.Fatalf("unexpected exit %d", code)
	}
	t.Cleanup(func() { exitFunc = origExit })

	redisStore := testutil.SetupMiniredisStore(t)
	pool := newMockPool(t)
	withMockPostgresStore(t, pool)
	withMockRedisStore(t, redisStore)
	withMigrationsHook(t, nil)

	t.Setenv("ENABLE_HSTS", "false")
	t.Setenv("JWT_PRIVATE_KEY", testsecrets.TestJWTPrivateKeyPEM)
	t.Setenv("DATABASE_URL", "postgres://mock/mock?sslmode=disable")
	t.Setenv("REDIS_URL", redisStore.Client().Options().Addr)
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)

	prevEnv := serverEnv
	serverEnv = appConfig.Load()
	serverEnv.EnableHSTS = false
	serverEnv.MigrationsDir = "migrations"
	serverEnv.AllowedOrigins = "http://localhost"
	t.Cleanup(func() { serverEnv = prevEnv })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	t.Setenv("PORT", strconv.Itoa(port))

	sigCh := make(chan os.Signal, 1)
	prev := shutdownSignals
	shutdownSignals = func() <-chan os.Signal { return sigCh }
	t.Cleanup(func() { shutdownSignals = prev })

	done := make(chan struct{})
	go func() {
		_ = Run()
		close(done)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/health/live")
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	sigCh <- syscall.SIGTERM

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("Run did not complete")
	}
}

func TestRunServer_FullHappyPath(t *testing.T) {
	dbURL := tryPostgresURL(t)
	redisStore := testutil.SetupMiniredisStore(t)
	addr := redisStore.Client().Options().Addr

	t.Setenv("ENABLE_HSTS", "false")
	t.Setenv("JWT_PRIVATE_KEY", testsecrets.TestJWTPrivateKeyPEM)
	t.Setenv("DATABASE_URL", dbURL)
	t.Setenv("REDIS_URL", addr)
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)

	prevEnv := serverEnv
	serverEnv = appConfig.Load()
	serverEnv.EnableHSTS = false
	serverEnv.MigrationsDir = "migrations"
	serverEnv.AllowedOrigins = "http://localhost"
	t.Cleanup(func() { serverEnv = prevEnv })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	t.Setenv("PORT", strconv.Itoa(port))

	sigCh := make(chan os.Signal, 1)
	prev := shutdownSignals
	shutdownSignals = func() <-chan os.Signal { return sigCh }
	t.Cleanup(func() { shutdownSignals = prev })

	done := make(chan error, 1)
	go func() { done <- runServer(slog.Default()) }()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/health/live")
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

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
	t.Setenv("ENABLE_HSTS", "false")
	t.Setenv("JWT_PRIVATE_KEY", testsecrets.TestJWTPrivateKeyPEM)
	t.Setenv("DATABASE_URL", "postgres://invalid-host:59999/nodb?sslmode=disable&connect_timeout=1")
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
	t.Setenv("REDIS_URL", "127.0.0.1:6379")

	prevEnv := serverEnv
	t.Cleanup(func() { serverEnv = prevEnv })
	serverEnv = appConfig.Load()
	serverEnv.EnableHSTS = false

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

	origRedis := newRedisStoreFn
	newRedisStoreFn = func(_ string, _ appConfig.TimeoutConfig, _ ...store.Deps) (*store.RedisStore, error) {
		return nil, errors.New("redis unavailable")
	}
	t.Cleanup(func() { newRedisStoreFn = origRedis })

	withMigrationsHook(t, nil)

	t.Setenv("ENABLE_HSTS", "false")
	t.Setenv("JWT_PRIVATE_KEY", testsecrets.TestJWTPrivateKeyPEM)
	t.Setenv("DATABASE_URL", "postgres://mock/mock?sslmode=disable")
	t.Setenv("REDIS_URL", "127.0.0.1:59999")
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)

	prevEnv := serverEnv
	t.Cleanup(func() { serverEnv = prevEnv })
	serverEnv = appConfig.Load()
	serverEnv.EnableHSTS = false
	serverEnv.MigrationsDir = "migrations"

	err := runServer(slog.Default())
	if err == nil {
		t.Fatal("expected runServer to fail on invalid redis")
	}
}

func TestWaitForShutdown_Graceful(t *testing.T) {
	sigCh := make(chan os.Signal, 1)
	prev := shutdownSignals
	shutdownSignals = func() <-chan os.Signal { return sigCh }
	t.Cleanup(func() { shutdownSignals = prev })

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

	sigCh := make(chan os.Signal, 1)
	prev := shutdownSignals
	shutdownSignals = func() <-chan os.Signal { return sigCh }
	t.Cleanup(func() { shutdownSignals = prev })

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
	withMockRedisStore(t, redisStore)
	withMigrationsHook(t, nil)

	t.Setenv("ENABLE_HSTS", "false")
	t.Setenv("JWT_PRIVATE_KEY", testsecrets.TestJWTPrivateKeyPEM)
	t.Setenv("DATABASE_URL", "postgres://mock/mock?sslmode=disable")
	t.Setenv("REDIS_URL", redisStore.Client().Options().Addr)
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)

	prevEnv := serverEnv
	serverEnv = appConfig.Load()
	serverEnv.EnableHSTS = false
	serverEnv.MigrationsDir = "migrations"
	serverEnv.AllowedOrigins = "http://localhost"
	t.Cleanup(func() { serverEnv = prevEnv })

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
