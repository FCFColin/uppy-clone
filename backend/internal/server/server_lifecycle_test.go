package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	appConfig "github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/handler"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/testutil"
)

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
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
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

func TestInitDB_InvalidURL(t *testing.T) {
	cfg := &handler.Config{DatabaseURL: "postgres://invalid-host:59999/nodb?sslmode=disable&connect_timeout=1"}
	_, err := initDB(cfg, appConfig.DefaultTimeoutConfig())
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
	_, err := initDB(cfg, appConfig.DefaultTimeoutConfig())
	if err == nil {
		t.Fatal("expected migration error when migrations path is invalid")
	}
}

func TestRunServer_InvalidDatabase(t *testing.T) {
	t.Setenv("ENABLE_HSTS", "false")
	t.Setenv("JWT_SECRET", "dev-only-test-secret-32bytes!!")
	t.Setenv("DATABASE_URL", "postgres://invalid-host:59999/nodb?sslmode=disable&connect_timeout=1")
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
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
		startServer(r, &handler.Config{Port: "999999"})
		time.Sleep(500 * time.Millisecond)
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
	prev := metricsCollectInterval
	metricsCollectInterval = 15 * time.Millisecond
	t.Cleanup(func() { metricsCollectInterval = prev })

	redisStore := testutil.SetupMiniredisStore(t)
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
	hub := game.NewHub(nil, redisStore, appConfig.DefaultTimeoutConfig(), 10, 8, nil)
	if _, err := hub.CreateRoom(context.Background()); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	startMetricsCollector(ctx, hub, db, redisStore)
	time.Sleep(metricsCollectInterval + 25*time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)
}

func TestStartMetricsCollector_UpdatesOnTick(t *testing.T) {
	if testing.Short() {
		t.Skip("metrics tick interval is 15s; skip in -short")
	}

	redisStore := testutil.SetupMiniredisStore(t)
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
	hub := game.NewHub(nil, redisStore, appConfig.DefaultTimeoutConfig(), 10, 8, nil)

	ctx, cancel := context.WithCancel(context.Background())
	startMetricsCollector(ctx, hub, db, redisStore)
	time.Sleep(appConfig.MetricsInterval + time.Second)
	cancel()
	time.Sleep(50 * time.Millisecond)
}

func TestInitRedis_Success(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	addr := redisStore.Client().Options().Addr
	cfg := &handler.Config{RedisURL: addr}
	got, err := initRedis(cfg, appConfig.DefaultTimeoutConfig())
	if err != nil {
		t.Fatalf("initRedis: %v", err)
	}
	defer got.Close()
}

func TestInitRedis_InvalidURL(t *testing.T) {
	cfg := &handler.Config{RedisURL: "redis://invalid-host:59999"}
	_, err := initRedis(cfg, appConfig.TimeoutConfig{
		RedisConnectTimeout: time.Second,
		RedisReadTimeout:    time.Second,
		RedisWriteTimeout:   time.Second,
	})
	if err == nil {
		t.Fatal("expected initRedis error")
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
	broadcaster := game.NewPubSubBroadcaster(redisStore.Client())

	serverEnv = &appConfig.Env{MaxWSConnections: 100, MaxPlayersPerRoom: 8}
	t.Cleanup(func() { serverEnv = nil })

	hub := initHub(db, redisStore, appConfig.DefaultTimeoutConfig(), broadcaster)
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
	broadcaster := game.NewPubSubBroadcaster(redisStore.Client())

	serverEnv = &appConfig.Env{MaxWSConnections: 100, MaxPlayersPerRoom: 8}
	t.Cleanup(func() { serverEnv = nil })

	hub := initHub(db, redisStore, appConfig.DefaultTimeoutConfig(), broadcaster)
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
	cfg := &handler.Config{ResendAPIKey: "re_test", EmailFrom: "test@example.com"}
	startWorkers(ctx, cfg, redisStore, db, appConfig.DefaultTimeoutConfig())
}

func TestStartMetricsCollector_Cancel(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
	hub := game.NewHub(nil, redisStore, appConfig.DefaultTimeoutConfig(), 10, 8, nil)

	ctx, cancel := context.WithCancel(context.Background())
	startMetricsCollector(ctx, hub, db, redisStore)
	cancel()
	time.Sleep(50 * time.Millisecond)
}

func TestRunServer_RedisInitFail(t *testing.T) {
	dbURL := tryPostgresURL(t)
	t.Setenv("ENABLE_HSTS", "false")
	t.Setenv("JWT_SECRET", "dev-only-test-secret-32bytes!!")
	t.Setenv("DATABASE_URL", dbURL)
	t.Setenv("REDIS_URL", "127.0.0.1:59999")
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

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

func TestRunServer_InitCryptoFail(t *testing.T) {
	t.Setenv("ENABLE_HSTS", "false")
	t.Setenv("JWT_SECRET", "dev-only-test-secret-32bytes!!")
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
	cmd.Env = append(os.Environ(), "TEST_RUN_SERVER_SUBPROCESS=1")
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); !ok {
		t.Fatalf("expected exit error, got %v", err)
	} else if exitErr.ExitCode() == 0 {
		t.Fatal("runServer should not succeed with empty env")
	}
}

func TestRun_ExitsOnFailure(t *testing.T) {
	if os.Getenv("TEST_SERVER_RUN_SUBPROCESS") == "1" {
		serverEnv = &appConfig.Env{}
		Run()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRun_ExitsOnFailure$", "-test.v")
	cmd.Env = append(os.Environ(), "TEST_SERVER_RUN_SUBPROCESS=1")
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("Run should exit 1, got %v", err)
	}
}

func TestWaitForShutdown_Graceful(t *testing.T) {
	sigCh := make(chan os.Signal, 1)
	prev := shutdownSignals
	shutdownSignals = func() <-chan os.Signal { return sigCh }
	t.Cleanup(func() { shutdownSignals = prev })

	redisStore := testutil.SetupMiniredisStore(t)
	broadcaster := game.NewPubSubBroadcaster(redisStore.Client())
	hub := game.NewHub(nil, redisStore, appConfig.DefaultTimeoutConfig(), 10, 8, broadcaster)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		waitForShutdown(srv.Config, cancel, hub, broadcaster)
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

	prevEnv := serverEnv
	serverEnv = &appConfig.Env{
		MaxWSConnections:  100,
		MaxPlayersPerRoom: 8,
		AllowedOrigins:    "http://localhost",
		AdminJWTSecret:    "test-admin-jwt-secret-padded-32bytes!",
	}
	t.Cleanup(func() { serverEnv = prevEnv })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	cfg := &handler.Config{
		Port:      strconv.Itoa(port),
		JWTSecret: "test-secret-key-padded-to-32-bytes!!",
	}
	timeouts := appConfig.DefaultTimeoutConfig()
	ctx := context.Background()

	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, cfg, timeouts, db, redisStore)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://127.0.0.1:" + cfg.Port + "/health/live")
		if err == nil {
			resp.Body.Close()
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
