package migrateutil

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func tryPostgresConnString(t *testing.T) string {
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

func TestFileSourceURL(t *testing.T) {
	dir := t.TempDir()
	got, err := FileSourceURL(dir)
	if err != nil {
		t.Fatalf("FileSourceURL: %v", err)
	}
	if !strings.HasPrefix(got, "file://") {
		t.Fatalf("expected file:// prefix, got %q", got)
	}
	if strings.Contains(got, ":///") {
		t.Fatalf("Windows-safe URL must not use file:///, got %q", got)
	}
	abs, _ := filepath.Abs(dir)
	if !strings.Contains(got, filepath.ToSlash(abs)) {
		t.Fatalf("URL should contain absolute migrations dir, got %q", got)
	}
}

func TestEnsureDBRoles_ConnectError(t *testing.T) {
	err := EnsureDBRoles(t.Context(), "postgres://invalid:5432/nodb?sslmode=disable")
	if err == nil {
		t.Fatal("expected connect error")
	}
}

func TestRunMigrations_InvalidConn(t *testing.T) {
	err := RunMigrations(context.Background(), "postgres://invalid:5432/nodb?sslmode=disable", t.TempDir())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFileSourceURL_InvalidPath(t *testing.T) {
	_, err := FileSourceURL(string([]byte{0}))
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

type fakePgxConn struct {
	execErr error
}

func (f *fakePgxConn) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	if f.execErr != nil {
		return pgconn.CommandTag{}, f.execErr
	}
	return pgconn.NewCommandTag("DO"), nil
}

func (f *fakePgxConn) Close(_ context.Context) error { return nil }

type fakeMigrator struct {
	upErr error
}

func (f *fakeMigrator) Up() error { return f.upErr }

func TestEnsureDBRoles_SuccessMocked(t *testing.T) {
	prev := pgxConnect
	pgxConnect = func(_ context.Context, _ string) (pgxExecer, error) {
		return &fakePgxConn{}, nil
	}
	t.Cleanup(func() { pgxConnect = prev })

	if err := EnsureDBRoles(context.Background(), "postgres://unused"); err != nil {
		t.Fatalf("EnsureDBRoles: %v", err)
	}
}

func TestEnsureDBRoles_ExecError(t *testing.T) {
	prev := pgxConnect
	pgxConnect = func(_ context.Context, _ string) (pgxExecer, error) {
		return &fakePgxConn{execErr: errors.New("exec failed")}, nil
	}
	t.Cleanup(func() { pgxConnect = prev })

	if err := EnsureDBRoles(context.Background(), "postgres://unused"); err == nil {
		t.Fatal("expected exec error")
	}
}

func TestRunMigrations_SuccessMocked(t *testing.T) {
	prevConnect := pgxConnect
	prevMigrate := newMigrateRunner
	pgxConnect = func(_ context.Context, _ string) (pgxExecer, error) {
		return &fakePgxConn{}, nil
	}
	newMigrateRunner = func(_, _ string) (migrateRunner, error) {
		return &fakeMigrator{}, nil
	}
	t.Cleanup(func() {
		pgxConnect = prevConnect
		newMigrateRunner = prevMigrate
	})

	if err := RunMigrations(context.Background(), "postgres://unused", t.TempDir()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
}

func TestRunMigrations_NoChange(t *testing.T) {
	prevConnect := pgxConnect
	prevMigrate := newMigrateRunner
	pgxConnect = func(_ context.Context, _ string) (pgxExecer, error) {
		return &fakePgxConn{}, nil
	}
	newMigrateRunner = func(_, _ string) (migrateRunner, error) {
		return &fakeMigrator{upErr: migrate.ErrNoChange}, nil
	}
	t.Cleanup(func() {
		pgxConnect = prevConnect
		newMigrateRunner = prevMigrate
	})

	if err := RunMigrations(context.Background(), "postgres://unused", t.TempDir()); err != nil {
		t.Fatalf("RunMigrations ErrNoChange: %v", err)
	}
}

func TestRunMigrations_MigrateInitError(t *testing.T) {
	prevConnect := pgxConnect
	prevMigrate := newMigrateRunner
	pgxConnect = func(_ context.Context, _ string) (pgxExecer, error) {
		return &fakePgxConn{}, nil
	}
	newMigrateRunner = func(_, _ string) (migrateRunner, error) {
		return nil, errors.New("init failed")
	}
	t.Cleanup(func() {
		pgxConnect = prevConnect
		newMigrateRunner = prevMigrate
	})

	err := RunMigrations(context.Background(), "postgres://unused", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "migrate init") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunMigrations_MigrateUpError(t *testing.T) {
	prevConnect := pgxConnect
	prevMigrate := newMigrateRunner
	pgxConnect = func(_ context.Context, _ string) (pgxExecer, error) {
		return &fakePgxConn{}, nil
	}
	newMigrateRunner = func(_, _ string) (migrateRunner, error) {
		return &fakeMigrator{upErr: errors.New("up failed")}, nil
	}
	t.Cleanup(func() {
		pgxConnect = prevConnect
		newMigrateRunner = prevMigrate
	})

	err := RunMigrations(context.Background(), "postgres://unused", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "migrate up") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureDBRoles_Success(t *testing.T) {
	connStr := tryPostgresConnString(t)
	if err := EnsureDBRoles(t.Context(), connStr); err != nil {
		t.Fatalf("EnsureDBRoles: %v", err)
	}
	// Idempotent: second call should still succeed.
	if err := EnsureDBRoles(t.Context(), connStr); err != nil {
		t.Fatalf("EnsureDBRoles second call: %v", err)
	}
}

func TestRunMigrations_SuccessEmptyDir(t *testing.T) {
	connStr := tryPostgresConnString(t)
	emptyDir := t.TempDir()
	if err := RunMigrations(context.Background(), connStr, emptyDir); err != nil {
		t.Fatalf("RunMigrations empty dir: %v", err)
	}
}

func TestRunMigrations_WithMigrationsDir(t *testing.T) {
	connStr := tryPostgresConnString(t)
	migrationsDir := filepath.Join("..", "..", "migrations")
	abs, err := filepath.Abs(migrationsDir)
	if err != nil {
		t.Fatalf("abs migrations: %v", err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Skipf("migrations dir unavailable: %v", err)
	}
	if err := RunMigrations(context.Background(), connStr, abs); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
}

func TestRunMigrations_InvalidMigrationsPath(t *testing.T) {
	connStr := tryPostgresConnString(t)
	err := RunMigrations(context.Background(), connStr, string([]byte{0}))
	if err == nil {
		t.Fatal("expected migrate source path error")
	}
	if !strings.Contains(err.Error(), "migrate source path") {
		t.Fatalf("unexpected error: %v", err)
	}
}
