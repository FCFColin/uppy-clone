package migrateutil

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
)

const ensureDBRolesSQL = `
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_user') THEN
    CREATE ROLE app_user WITH LOGIN PASSWORD 'change_in_production' NOCREATEDB NOCREATEROLE NOSUPERUSER;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'migrator') THEN
    CREATE ROLE migrator WITH LOGIN PASSWORD 'change_in_production' NOCREATEDB NOCREATEROLE NOSUPERUSER;
  END IF;
END $$;
`

// FileSourceURL builds a golang-migrate file source URL that works on Windows.
// Use "file://" + forward-slash absolute path (not file:///D:/...) so migrate's
// parseURL yields a valid OS path on Windows.
func FileSourceURL(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return "file://" + filepath.ToSlash(abs), nil
}

// EnsureDBRoles creates roles required by migration 000009 when missing.
// Production Docker runs docker/init-scripts/01-create-roles.sql at init time.
func EnsureDBRoles(ctx context.Context, connString string) error {
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		return fmt.Errorf("connect for roles: %w", err)
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, ensureDBRolesSQL); err != nil {
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
	m, err := migrate.New(source, connString)
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}
