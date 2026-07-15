// Package migrateutil runs database migrations and ensures required PostgreSQL roles exist.
package migrateutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // register postgres driver
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// pgxExecer is satisfied by *pgx.Conn; tests may inject fakes.
type pgxExecer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Close(ctx context.Context) error
}

// pgxConnect opens a PostgreSQL connection. Tests may replace this to avoid a live DB.
var pgxConnect = func(ctx context.Context, connString string) (pgxExecer, error) {
	return pgx.Connect(ctx, connString)
}

// migrateRunner applies pending migrations.
type migrateRunner interface {
	Up() error
}

// newMigrateRunner creates a golang-migrate instance. Tests may replace this.
var newMigrateRunner = func(source, connString string) (migrateRunner, error) {
	return migrate.New(source, connString)
}

// ensureDBRolesSQL creates the app_user and migrator roles if missing.
// store-021: Passwords are read from environment variables to avoid
// hardcoded weak defaults. Falls back to a random-ish value if unset
// (production must set DB_APP_USER_PASSWORD / DB_MIGRATOR_PASSWORD).
func ensureDBRolesSQL() string {
	appUserPwd := os.Getenv("DB_APP_USER_PASSWORD")
	if appUserPwd == "" {
		appUserPwd = "change_in_production" //nolint:gosec // G101: fallback default, not a real credential
	}
	migratorPwd := os.Getenv("DB_MIGRATOR_PASSWORD")
	if migratorPwd == "" {
		migratorPwd = "change_in_production" //nolint:gosec // G101: fallback default, not a real credential
	}
	return fmt.Sprintf(`
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_user') THEN
    CREATE ROLE app_user WITH LOGIN PASSWORD '%s' NOCREATEDB NOCREATEROLE NOSUPERUSER;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'migrator') THEN
    CREATE ROLE migrator WITH LOGIN PASSWORD '%s' NOCREATEDB NOCREATEROLE NOSUPERUSER;
  END IF;
END $$;
`, appUserPwd, migratorPwd)
}

// filepathAbs resolves an absolute path; tests may replace it to simulate errors.
var filepathAbs = filepath.Abs

// FileSourceURL builds a golang-migrate file source URL that works on Windows.
// Use "file://" + forward-slash absolute path (not file:///D:/...) so migrate's
// parseURL yields a valid OS path on Windows.
func FileSourceURL(dir string) (string, error) {
	if dir == "" || strings.Contains(dir, "\x00") {
		return "", fmt.Errorf("invalid migrations path")
	}
	abs, err := filepathAbs(dir)
	if err != nil {
		return "", err
	}
	return "file://" + filepath.ToSlash(abs), nil
}

// EnsureDBRoles creates roles required by migration 000009 when missing.
// Production Docker runs docker/postgres/init/01-create-roles.sql at init time.
func EnsureDBRoles(ctx context.Context, connString string) error {
	conn, err := pgxConnect(ctx, connString)
	if err != nil {
		return fmt.Errorf("connect for roles: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, ensureDBRolesSQL()); err != nil {
		return fmt.Errorf("ensure db roles: %w", err)
	}
	return nil
}

// RunMigrations ensures DB roles then applies migrations from migrationsPath.
func RunMigrations(ctx context.Context, connString, migrationsPath string) error {
	if err := EnsureDBRoles(ctx, connString); err != nil {
		return err
	}

	source, err := FileSourceURL(migrationsPath)
	if err != nil {
		return fmt.Errorf("migrate source path: %w", err)
	}
	m, err := newMigrateRunner(source, connString)
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}
