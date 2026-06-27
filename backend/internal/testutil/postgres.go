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

// SetupPostgresConn starts PostgreSQL via testcontainers and returns a pgx connection.
func SetupPostgresConn(t *testing.T) (*pgx.Conn, string) {
	t.Helper()
	skipIfShort(t)

	ctx := context.Background()
	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
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
	return conn, connStr
}

// DefaultTimeouts returns standard timeout config for integration tests.
func DefaultTimeouts() config.TimeoutConfig {
	return config.TimeoutConfig{
		PGConnectTimeout: 10 * time.Second,
		PGQueryTimeout:   10 * time.Second,
		PGRequestTimeout: 30 * time.Second,
	}
}

// SetupPostgresPool starts PostgreSQL via testcontainers and returns a connected pool.
func SetupPostgresPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	skipIfShort(t)

	ctx := context.Background()
	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
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

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// SetupPostgresStore starts PostgreSQL, runs migrations, and returns a PostgresStore.
func SetupPostgresStore(t *testing.T) *store.PostgresStore {
	t.Helper()
	skipIfShort(t)

	ctx := context.Background()
	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
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

	timeouts := DefaultTimeouts()
	db, err := store.NewPostgresStore(connStr, timeouts)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	t.Cleanup(db.Close)

	if err := db.RunMigrations(MigrationsDir(t)); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	return db
}

func skipIfShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
}
