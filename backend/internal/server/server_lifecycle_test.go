package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pashagolub/pgxmock/v4"
	appConfig "github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/handler"
	"github.com/uppy-clone/backend/internal/store"
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

func TestInitProfiling(t *testing.T) {
	initProfiling() // disabled by default
	t.Setenv("ENABLE_PYROSCOPE", "true")
	t.Setenv("PYROSCOPE_SERVER_ADDRESS", "http://localhost:4040")
	initProfiling()
}

func TestInitProfiling_NoAddress(t *testing.T) {
	t.Setenv("ENABLE_PYROSCOPE", "true")
	t.Setenv("PYROSCOPE_SERVER_ADDRESS", "")
	initProfiling() // should return early without panicking
}

func TestInitDB_MigrationWarnEmptyDatabaseURL(t *testing.T) {
	poolCfg, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:1/dbname?sslmode=disable")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	poolCfg.ConnConfig.ConnectTimeout = 500 * time.Millisecond
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	origPG := newPostgresStoreFn
	newPostgresStoreFn = func(_ string, _ appConfig.TimeoutConfig, _ ...store.Deps) (*store.PostgresStore, error) {
		return store.NewPostgresStoreWithPool(pool), nil
	}
	t.Cleanup(func() { newPostgresStoreFn = origPG })

	origRun := store.SetRunMigrationsHook(func(context.Context, string, string) error {
		return errors.New("migration failed")
	})
	t.Cleanup(origRun)

	cfg := &handler.Config{DatabaseURL: ""}
	db, err := initDB(cfg, appConfig.DefaultTimeoutConfig(), store.DefaultDeps())
	if err != nil {
		t.Fatalf("initDB should warn-not-fail when DatabaseURL empty: %v", err)
	}
	defer db.Close()
}

func TestInitDB_MigrationFailsNonEmptyURL(t *testing.T) {
	poolCfg, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:1/dbname?sslmode=disable")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	poolCfg.ConnConfig.ConnectTimeout = 500 * time.Millisecond
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	origPG := newPostgresStoreFn
	newPostgresStoreFn = func(_ string, _ appConfig.TimeoutConfig, _ ...store.Deps) (*store.PostgresStore, error) {
		return store.NewPostgresStoreWithPool(pool), nil
	}
	t.Cleanup(func() { newPostgresStoreFn = origPG })

	origRun := store.SetRunMigrationsHook(func(context.Context, string, string) error {
		return errors.New("migration failed")
	})
	t.Cleanup(origRun)

	cfg := &handler.Config{DatabaseURL: "postgres://mock/mock?sslmode=disable"}
	_, err = initDB(cfg, appConfig.DefaultTimeoutConfig(), store.DefaultDeps())
	if err == nil {
		t.Fatal("expected migration error when DatabaseURL set")
	}
}

func TestWaitForShutdown_AlreadyClosedServer(t *testing.T) {
	sigCh := make(chan os.Signal, 1)
	prev := shutdownSignals
	shutdownSignals = func() <-chan os.Signal { return sigCh }
	t.Cleanup(func() { shutdownSignals = prev })

	redisStore := testutil.SetupMiniredisStore(t)
	hub := game.NewHub(nil, redisStore, appConfig.DefaultTimeoutConfig(), 10, 8)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.Close()

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

func TestWaitForShutdown_GracefulNoRedis(t *testing.T) {
	sigCh := make(chan os.Signal, 1)
	prev := shutdownSignals
	shutdownSignals = func() <-chan os.Signal { return sigCh }
	t.Cleanup(func() { shutdownSignals = prev })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	hub := game.NewHub(nil, nil, appConfig.DefaultTimeoutConfig(), 0, 0)

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
		t.Fatal("expected context cancelled")
	}
}

func TestInitDB_InvalidURL(t *testing.T) {
	cfg := &handler.Config{DatabaseURL: "postgres://invalid-host:59999/nodb?sslmode=disable&connect_timeout=1"}
	_, err := initDB(cfg, appConfig.DefaultTimeoutConfig(), store.DefaultDeps())
	if err == nil {
		t.Fatal("expected initDB error")
	}
}

func TestInitDB_MigrationFails(t *testing.T) {
	dbURL := tryPostgresURL(t)
	prevEnv := serverEnv
	serverEnv = &appConfig.Env{MigrationsDir: string([]byte{0})}
	t.Cleanup(func() { serverEnv = prevEnv })

	cfg := &handler.Config{DatabaseURL: dbURL}
	_, err := initDB(cfg, appConfig.DefaultTimeoutConfig(), store.DefaultDeps())
	if err == nil {
		t.Fatal("expected migration error when migrations path is invalid")
	}
}

func TestInitDB_SuccessMocked(t *testing.T) {
	poolCfg, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:1/dbname?sslmode=disable")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	poolCfg.ConnConfig.ConnectTimeout = 500 * time.Millisecond
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	origPG := newPostgresStoreFn
	newPostgresStoreFn = func(_ string, _ appConfig.TimeoutConfig, _ ...store.Deps) (*store.PostgresStore, error) {
		return store.NewPostgresStoreWithPool(pool), nil
	}
	t.Cleanup(func() { newPostgresStoreFn = origPG })

	restoreMig := store.SetRunMigrationsHook(func(context.Context, string, string) error { return nil })
	t.Cleanup(restoreMig)

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

func TestInitDB_MigrationsDirFromEnv(t *testing.T) {
	poolCfg, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:1/dbname?sslmode=disable")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	poolCfg.ConnConfig.ConnectTimeout = 500 * time.Millisecond
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	origPG := newPostgresStoreFn
	newPostgresStoreFn = func(_ string, _ appConfig.TimeoutConfig, _ ...store.Deps) (*store.PostgresStore, error) {
		return store.NewPostgresStoreWithPool(pool), nil
	}
	t.Cleanup(func() { newPostgresStoreFn = origPG })

	origRun := store.SetRunMigrationsHook(func(_ context.Context, _, path string) error {
		if path != "custom-migrations" {
			t.Fatalf("migrations path = %q, want custom-migrations", path)
		}
		return nil
	})
	t.Cleanup(origRun)

	prevEnv := serverEnv
	serverEnv = &appConfig.Env{MigrationsDir: "custom-migrations"}
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

	poolCfg, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:1/dbname?sslmode=disable")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	poolCfg.ConnConfig.ConnectTimeout = 500 * time.Millisecond
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	origPG := newPostgresStoreFn
	origRedis := newRedisStoreFn
	newPostgresStoreFn = func(_ string, _ appConfig.TimeoutConfig, _ ...store.Deps) (*store.PostgresStore, error) {
		return store.NewPostgresStoreWithPool(pool), nil
	}
	newRedisStoreFn = func(_ string, _ appConfig.TimeoutConfig, _ ...store.Deps) (*store.RedisStore, error) {
		return redisStore, nil
	}
	t.Cleanup(func() {
		newPostgresStoreFn = origPG
		newRedisStoreFn = origRedis
	})

	restoreMig := store.SetRunMigrationsHook(func(context.Context, string, string) error { return nil })
	t.Cleanup(restoreMig)

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

	origPG := newPostgresStoreFn
	origRedis := newRedisStoreFn
	origExit := exitFunc
	exitFunc = func(code int) {
		t.Fatalf("unexpected exit %d", code)
	}
	t.Cleanup(func() {
		newPostgresStoreFn = origPG
		newRedisStoreFn = origRedis
		exitFunc = origExit
	})

	redisStore := testutil.SetupMiniredisStore(t)

	poolCfg, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:1/dbname?sslmode=disable")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	poolCfg.ConnConfig.ConnectTimeout = 500 * time.Millisecond
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	newPostgresStoreFn = func(_ string, _ appConfig.TimeoutConfig, _ ...store.Deps) (*store.PostgresStore, error) {
		return store.NewPostgresStoreWithPool(pool), nil
	}
	newRedisStoreFn = func(_ string, _ appConfig.TimeoutConfig, _ ...store.Deps) (*store.RedisStore, error) {
		return redisStore, nil
	}
	restoreMig := store.SetRunMigrationsHook(func(context.Context, string, string) error { return nil })
	t.Cleanup(restoreMig)

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

func TestInitDB_Success(t *testing.T) {
	dbURL := tryPostgresURL(t)
	prevEnv := serverEnv
	serverEnv = &appConfig.Env{MigrationsDir: "migrations"}
	t.Cleanup(func() { serverEnv = prevEnv })

	cfg := &handler.Config{DatabaseURL: dbURL}
	db, err := initDB(cfg, appConfig.DefaultTimeoutConfig(), store.DefaultDeps())
	if err != nil {
		t.Fatalf("initDB: %v", err)
	}
	defer db.Close()
	if db.Pool() == nil {
		t.Fatal("expected non-nil pool")
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

func TestStartServer_ListenErrorExits(t *testing.T) {
	if os.Getenv("TEST_START_SERVER_LISTEN_ERROR") == "1" {
		r := chi.NewRouter()
		_, errCh := startServer(r, &handler.Config{Port: "999999"})
		if err := <-errCh; err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestStartServer_ListenErrorExits$", "-test.v")
	cmd.Env = append(os.Environ(), "TEST_START_SERVER_LISTEN_ERROR=1")
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); !ok {
		t.Fatalf("expected exit error, got %v", err)
	} else if exitErr.ExitCode() != 1 {
		t.Fatalf("startServer listen error should exit 1, got %d", exitErr.ExitCode())
	}
}

func TestStartMetricsCollector_TickInShort(t *testing.T) {
	serverLifecycleMu.Lock()
	t.Cleanup(func() { serverLifecycleMu.Unlock() })

	prev := metricsCollectInterval
	metricsCollectInterval = 15 * time.Millisecond
	t.Cleanup(func() { metricsCollectInterval = prev })

	redisStore := testutil.SetupMiniredisStore(t)
	cluster := store.NewRedisClusterFromStores(redisStore, nil)
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
	hub := game.NewHub(nil, redisStore, appConfig.DefaultTimeoutConfig(), 10, 8)

	ctx, cancel := context.WithCancel(context.Background())
	startMetricsCollector(ctx, hub, db, cluster)
	time.Sleep(metricsCollectInterval + 25*time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)
}

func TestStartMetricsCollector_UpdatesOnTick(t *testing.T) {
	if testing.Short() {
		t.Skip("metrics tick interval is 15s; skip in -short")
	}

	redisStore := testutil.SetupMiniredisStore(t)
	cluster := store.NewRedisClusterFromStores(redisStore, nil)
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
	hub := game.NewHub(nil, redisStore, appConfig.DefaultTimeoutConfig(), 10, 8)

	ctx, cancel := context.WithCancel(context.Background())
	startMetricsCollector(ctx, hub, db, cluster)
	time.Sleep(appConfig.MetricsInterval + time.Second)
	cancel()
	time.Sleep(50 * time.Millisecond)
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

func TestInitRedisCluster_InvalidURL(t *testing.T) {
	cfg := &handler.Config{RedisURL: "redis://invalid-host:59999"}
	_, err := initRedisCluster(cfg, appConfig.TimeoutConfig{
		RedisConnectTimeout: time.Second,
		RedisReadTimeout:    time.Second,
		RedisWriteTimeout:   time.Second,
	}, store.DefaultDeps())
	if err == nil {
		t.Fatal("expected initRedisCluster error")
	}
}

func TestInitHub_RestoreRoomsError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

	mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states").
		WithArgs(pgxmock.AnyArg()).
		WillReturnError(context.Canceled)

	db := store.NewPostgresStoreWithPool(mock)
	redisStore := testutil.SetupMiniredisStore(t)

	serverEnv = &appConfig.Env{MaxWSConnections: 100, MaxPlayersPerRoom: 8}
	t.Cleanup(func() { serverEnv = nil })

	hub := initHub(db, redisStore, appConfig.DefaultTimeoutConfig())
	if hub == nil {
		t.Fatal("initHub should return hub even when restore fails")
	}
}

func TestInitHub_RestoresRooms(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

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

func TestStartWorkers_Short(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	cfg := &handler.Config{ResendAPIKey: "re_test", EmailFrom: "test@example.com"}
	startWorkers(ctx, &wg, cfg, redisStore, db, appConfig.DefaultTimeoutConfig())
	cancel()
	wg.Wait()
}

func TestStartMetricsCollector_Cancel(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	cluster := store.NewRedisClusterFromStores(redisStore, nil)
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
	hub := game.NewHub(nil, redisStore, appConfig.DefaultTimeoutConfig(), 10, 8)

	ctx, cancel := context.WithCancel(context.Background())
	startMetricsCollector(ctx, hub, db, cluster)
	cancel()
	time.Sleep(50 * time.Millisecond)
}

func TestRunServer_RedisInitFail(t *testing.T) {
	serverLifecycleMu.Lock()
	t.Cleanup(func() { serverLifecycleMu.Unlock() })

	poolCfg, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:1/dbname?sslmode=disable")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	poolCfg.ConnConfig.ConnectTimeout = 500 * time.Millisecond
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	origPG := newPostgresStoreFn
	origRedis := newRedisStoreFn
	newPostgresStoreFn = func(_ string, _ appConfig.TimeoutConfig, _ ...store.Deps) (*store.PostgresStore, error) {
		return store.NewPostgresStoreWithPool(pool), nil
	}
	newRedisStoreFn = func(_ string, _ appConfig.TimeoutConfig, _ ...store.Deps) (*store.RedisStore, error) {
		return nil, errors.New("redis unavailable")
	}
	t.Cleanup(func() {
		newPostgresStoreFn = origPG
		newRedisStoreFn = origRedis
	})

	restoreMig := store.SetRunMigrationsHook(func(context.Context, string, string) error { return nil })
	t.Cleanup(restoreMig)

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

	err = runServer(slog.Default())
	if err == nil {
		t.Fatal("expected runServer to fail on invalid redis")
	}
}

func TestRunServer_InitCryptoFail(t *testing.T) {
	t.Setenv("ENABLE_HSTS", "false")
	t.Setenv("JWT_PRIVATE_KEY", testsecrets.TestJWTPrivateKeyPEM)
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("ENCRYPTION_KEY", "not-valid-hex")
	prevEnv := serverEnv
	t.Cleanup(func() { serverEnv = prevEnv })
	serverEnv = appConfig.Load()
	serverEnv.EnableHSTS = false

	err := runServer(slog.Default())
	if err == nil {
		t.Fatal("expected init crypto error")
	}
}

func TestRunServer_FailsWithoutEnv(t *testing.T) {
	if os.Getenv("TEST_RUN_SERVER_SUBPROCESS") == "1" {
		serverEnv = &appConfig.Env{}
		_ = runServer(slog.Default())
		os.Exit(0)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunServer_FailsWithoutEnv$", "-test.v")
	cmd.Env = []string{
		"TEST_RUN_SERVER_SUBPROCESS=1",
		"PATH=" + os.Getenv("PATH"),
		"SYSTEMROOT=" + os.Getenv("SYSTEMROOT"),
		"TEMP=" + os.Getenv("TEMP"),
		"TMP=" + os.Getenv("TMP"),
	}
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); !ok {
		t.Fatalf("expected exit error, got %v", err)
	} else if exitErr.ExitCode() == 0 {
		t.Fatal("runServer should not succeed with empty env")
	}
}

func TestRun_ExitsOnFailure(t *testing.T) {
	origExit := exitFunc
	var exitCode int
	exitFunc = func(code int) { exitCode = code }
	t.Cleanup(func() { exitFunc = origExit })

	prevEnv := serverEnv
	serverEnv = &appConfig.Env{}
	t.Cleanup(func() { serverEnv = prevEnv })

	_ = Run()
	if exitCode != 1 {
		t.Fatalf("Run should exit 1, got %d", exitCode)
	}
}

func TestRun_ExitsOnInitCryptoFailure(t *testing.T) {
	origExit := exitFunc
	var exitCode int
	exitFunc = func(code int) { exitCode = code }
	t.Cleanup(func() { exitFunc = origExit })

	t.Setenv("ENABLE_HSTS", "false")
	t.Setenv("JWT_PRIVATE_KEY", testsecrets.TestJWTPrivateKeyPEM)
	t.Setenv("ADMIN_JWT_PRIVATE_KEY", testsecrets.TestJWTPrivateKeyPEM)
	t.Setenv("DATABASE_URL", "postgres://mock/mock?sslmode=disable")
	t.Setenv("REDIS_URL", "127.0.0.1:6379")
	t.Setenv("ENCRYPTION_KEY", "not-valid-hex")
	t.Setenv("TRUSTED_PROXY_CIDRS", "127.0.0.1/32")

	prevEnv := serverEnv
	serverEnv = appConfig.Load()
	serverEnv.EnableHSTS = false
	t.Cleanup(func() { serverEnv = prevEnv })

	_ = Run()
	if exitCode != 1 {
		t.Fatalf("Run should exit 1 on init crypto failure, got %d", exitCode)
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

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states").
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id", "code", "state", "updated_at", "created_at"}))
	mock.ExpectExec("DELETE FROM users WHERE deleted_at IS NOT NULL").
		WithArgs(pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("DELETE 0"))

	db := store.NewPostgresStoreWithPool(mock)
	redisStore := testutil.SetupMiniredisStore(t)
	cluster := store.NewRedisClusterFromStores(redisStore, nil)

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

func TestShutdownSignals_ReturnsChannel(t *testing.T) {
	ch := shutdownSignals()
	if ch == nil {
		t.Fatal("shutdownSignals returned nil channel")
	}
}

func TestRunServer_TracerInitError(t *testing.T) {
	serverLifecycleMu.Lock()
	t.Cleanup(func() { serverLifecycleMu.Unlock() })

	prevTracer := initTracerFn
	initTracerFn = func(context.Context, string, string) (func(context.Context) error, error) {
		return nil, errors.New("tracer init failed")
	}
	t.Cleanup(func() { initTracerFn = prevTracer })

	redisStore := testutil.SetupMiniredisStore(t)
	poolCfg, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:1/dbname?sslmode=disable")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	poolCfg.ConnConfig.ConnectTimeout = 500 * time.Millisecond
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	origPG := newPostgresStoreFn
	origRedis := newRedisStoreFn
	newPostgresStoreFn = func(_ string, _ appConfig.TimeoutConfig, _ ...store.Deps) (*store.PostgresStore, error) {
		return store.NewPostgresStoreWithPool(pool), nil
	}
	newRedisStoreFn = func(_ string, _ appConfig.TimeoutConfig, _ ...store.Deps) (*store.RedisStore, error) {
		return redisStore, nil
	}
	t.Cleanup(func() {
		newPostgresStoreFn = origPG
		newRedisStoreFn = origRedis
	})
	t.Cleanup(store.SetRunMigrationsHook(func(context.Context, string, string) error { return nil }))

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

func TestRunServer_TracerShutdownError(t *testing.T) {
	serverLifecycleMu.Lock()
	t.Cleanup(func() { serverLifecycleMu.Unlock() })

	prevTracer := initTracerFn
	initTracerFn = func(context.Context, string, string) (func(context.Context) error, error) {
		return func(context.Context) error { return errors.New("tracer shutdown failed") }, nil
	}
	t.Cleanup(func() { initTracerFn = prevTracer })

	redisStore := testutil.SetupMiniredisStore(t)
	poolCfg, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:1/dbname?sslmode=disable")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	poolCfg.ConnConfig.ConnectTimeout = 500 * time.Millisecond
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	origPG := newPostgresStoreFn
	origRedis := newRedisStoreFn
	newPostgresStoreFn = func(_ string, _ appConfig.TimeoutConfig, _ ...store.Deps) (*store.PostgresStore, error) {
		return store.NewPostgresStoreWithPool(pool), nil
	}
	newRedisStoreFn = func(_ string, _ appConfig.TimeoutConfig, _ ...store.Deps) (*store.RedisStore, error) {
		return redisStore, nil
	}
	t.Cleanup(func() {
		newPostgresStoreFn = origPG
		newRedisStoreFn = origRedis
	})
	t.Cleanup(store.SetRunMigrationsHook(func(context.Context, string, string) error { return nil }))

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
	prevSig := shutdownSignals
	shutdownSignals = func() <-chan os.Signal { return sigCh }
	t.Cleanup(func() { shutdownSignals = prevSig })

	done := make(chan error, 1)
	go func() { done <- runServer(slog.Default()) }()
	time.Sleep(300 * time.Millisecond)
	sigCh <- syscall.SIGTERM
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runServer: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runServer did not shut down")
	}
}

func TestWaitForShutdown_ServerShutdownError(t *testing.T) {
	sigCh := make(chan os.Signal, 1)
	prevSig := shutdownSignals
	shutdownSignals = func() <-chan os.Signal { return sigCh }
	t.Cleanup(func() { shutdownSignals = prevSig })

	prevShutdown := serverShutdownFn
	serverShutdownFn = func(*http.Server, context.Context) error {
		return errors.New("shutdown failed")
	}
	t.Cleanup(func() { serverShutdownFn = prevShutdown })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	redisStore := testutil.SetupMiniredisStore(t)
	hub := game.NewHub(nil, redisStore, appConfig.DefaultTimeoutConfig(), 10, 8)

	_, cancel := context.WithCancel(context.Background())
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
}
