package testutil

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/store"
)

// PostgreSQL test strategy boundary (RO-048):
//
// The backend uses TWO complementary PostgreSQL test doubles. They serve
// distinct purposes and must NOT be merged — each guards a different class
// of regression.
//
// 1. pgxmock (github.com/pashagolub/pgxmock/v4) — UNIT tests.
//   - Entry point: store.newMockRepo factory in internal/store/postgres_mock_test.go.
//   - NO build tag → runs in the default `go test ./internal/...` suite.
//   - Mocks the pgx pool interface; fast, no Docker, CI-portable.
//   - Use for: repository method wiring, SQL argument mapping, error-path
//     branching, handler/auth/server logic that depends on store shape.
//   - Limitation: cannot catch SQL syntax errors, constraint violations,
//     or migration drift — that is the job of testcontainers.
//
// 2. testcontainers (github.com/testcontainers/testcontainers-go) — INTEGRATION tests.
//   - Entry point: testutil.SetupPostgres (this file).
//   - Build tag: `//go:build integration` → only runs with `-tags integration`.
//   - Spins up a real PostgreSQL 16 container via Docker.
//   - Use for: SQL correctness, migration compatibility (golang-migrate),
//     constraint/transaction behavior, end-to-end store + handler flows.
//   - Skips automatically when Docker is unavailable (t.Skipf).
//
// Decision rule for new tests:
//   - Testing pure Go logic with no need for real SQL → pgxmock (unit, no tag).
//   - Testing that SQL actually executes / migrations apply / constraints
//     fire → testcontainers (integration tag).

// PostgresTestEnv holds resources created by SetupPostgres.
type PostgresTestEnv struct {
	ConnStr string
	Conn    *pgx.Conn
	Pool    *pgxpool.Pool
	Store   *store.PostgresStore
}

type pgTestConfig struct {
	runMigrations bool
	wantPool      bool
	wantStore     bool
}

// PostgresOption configures SetupPostgres.
type PostgresOption func(*pgTestConfig)

// WithMigrations applies schema migrations to the test database.
func WithMigrations() PostgresOption {
	return func(c *pgTestConfig) { c.runMigrations = true }
}

// WithPool creates a pgxpool.Pool connected to the test database.
func WithPool() PostgresOption {
	return func(c *pgTestConfig) { c.wantPool = true }
}

// WithStore creates a PostgresStore connected to the test database.
func WithStore() PostgresOption {
	return func(c *pgTestConfig) { c.wantStore = true }
}

// SetupPostgres starts a PostgreSQL test container and optionally creates a
// pool, store, and/or applies migrations based on the provided options.
//
// At minimum a pgx.Conn and ConnStr are always returned. Use options to
// additionally create a Pool, Store, and/or run migrations:
//
//	env := testutil.SetupPostgres(t, testutil.WithStore(), testutil.WithMigrations())
//	db := env.Store
//
// Migration behavior matches the former functions:
//   - WithStore + WithMigrations: migrations via store.RunMigrations (golang-migrate)
//   - WithPool + WithMigrations (no store): migrations via RunMigrationsPGX (direct SQL)
//
// RO-036: consolidates SetupPostgresConn, SetupPostgresPool, SetupPostgresPoolMigrated,
// and SetupPostgresStore into a single function with options.
func SetupPostgres(t *testing.T, opts ...PostgresOption) *PostgresTestEnv {
	t.Helper()
	skipIfShort(t)

	cfg := pgTestConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	ctx := context.Background()
	connStr, conn := startPostgresContainer(t, ctx)
	env := &PostgresTestEnv{ConnStr: connStr, Conn: conn}

	if cfg.wantStore {
		setupPostgresStore(t, env, cfg)
	}
	if cfg.runMigrations && !cfg.wantStore {
		ensureTestRoles(t, conn)
		RunMigrationsPGX(t, conn, BackendMigrationsDir(t))
	}
	if cfg.wantPool {
		setupPostgresPool(t, env, ctx, connStr)
	}
	return env
}

// startPostgresContainer launches a PostgreSQL 16 test container, waits for it
// to be ready, and returns the connection string plus a live pgx.Conn. The
// container and conn are registered for t.Cleanup. If Docker is unavailable the
// test is skipped via t.Skipf.
func startPostgresContainer(t *testing.T, ctx context.Context) (string, *pgx.Conn) {
	t.Helper()
	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine@sha256:fd1e8d0274f13f5a03a2673a207b28e14823c2f2efc3ca4bb4197c8a9f841bdc",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		t.Skipf("postgres container unavailable: %v", err)
	}
	t.Cleanup(func() { _ = pgContainer.Terminate(ctx) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		t.Fatalf("pgx.Connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })
	return connStr, conn
}

// setupPostgresStore creates a PostgresStore on env and optionally applies
// migrations via store.RunMigrations (golang-migrate).
func setupPostgresStore(t *testing.T, env *PostgresTestEnv, cfg pgTestConfig) {
	t.Helper()
	timeouts := DefaultTimeouts()
	db, err := store.NewPostgresStore(env.ConnStr, timeouts)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	t.Cleanup(db.Close)
	if cfg.runMigrations {
		if err := db.RunMigrations(MigrationsDir(t)); err != nil {
			t.Fatalf("RunMigrations: %v", err)
		}
	}
	env.Store = db
}

// setupPostgresPool creates a pgxpool.Pool on env.
func setupPostgresPool(t *testing.T, env *PostgresTestEnv, ctx context.Context, connStr string) {
	t.Helper()
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	env.Pool = pool
}

// MigrationsDir resolves backend/migrations from caller location.
func MigrationsDir(t *testing.T) string {
	t.Helper()
	return BackendMigrationsDir(t)
}

// BackendMigrationsDir returns the absolute path to backend/migrations.
func BackendMigrationsDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine caller path")
	}
	dir := filepath.Join(filepath.Dir(filename), "..", "..", "migrations")
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("resolve migrations path: %v", err)
	}
	return abs
}

// RunMigrationsPGX applies *.up.sql files in order, optionally skipping name prefixes.
func RunMigrationsPGX(t *testing.T, conn *pgx.Conn, migrationsDir string, skipPrefixes ...string) {
	t.Helper()
	ctx := context.Background()
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	var upFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".up.sql") {
			upFiles = append(upFiles, entry.Name())
		}
	}
	sort.Strings(upFiles)
skipCheck:
	for _, name := range upFiles {
		for _, prefix := range skipPrefixes {
			if strings.HasPrefix(name, prefix) {
				continue skipCheck
			}
		}
		content, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if _, err := conn.Exec(ctx, string(content)); err != nil {
			t.Fatalf("exec %s: %v", name, err)
		}
	}
}

// DefaultTimeouts returns standard timeout config for integration tests.
func DefaultTimeouts() config.TimeoutConfig {
	return config.TimeoutConfig{
		PGConnectTimeout: 10 * time.Second,
		PGQueryTimeout:   10 * time.Second,
		PGRequestTimeout: 30 * time.Second,
	}
}

// ensureTestRoles creates the app_user and migrator roles required by
// migration 000009. The testcontainers postgres user is a superuser, so
// CREATE ROLE and GRANT succeed. This allows 000009 to run in tests.
// misc-031: pre-create DB roles so migration 000009 (GRANT statements) runs in tests without skipping.
func ensureTestRoles(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	ctx := context.Background()
	_, err := conn.Exec(ctx, `
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_user') THEN
    CREATE ROLE app_user WITH LOGIN PASSWORD 'test' NOCREATEDB NOCREATEROLE NOSUPERUSER;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'migrator') THEN
    CREATE ROLE migrator WITH LOGIN PASSWORD 'test' NOCREATEDB NOCREATEROLE NOSUPERUSER;
  END IF;
END $$;
`)
	if err != nil {
		t.Fatalf("ensure test roles: %v", err)
	}
}

func skipIfShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
}
