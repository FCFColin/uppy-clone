package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRunMigrate_ConnectionFailure(t *testing.T) {
	_, err := runMigrate("postgres://invalid:invalid@127.0.0.1:1/nodb?sslmode=disable")
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "connect") {
		t.Fatalf("error = %v", err)
	}
}

func TestMigrationStatusAfterLoad_BcryptShortCircuit(t *testing.T) {
	hashed := "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
	status, done := migrationStatusAfterLoad(hashed)
	if !done {
		t.Fatal("expected bcrypt password to short-circuit migration")
	}
	if status != "already bcrypt" {
		t.Fatalf("status = %q", status)
	}

	status, done = migrationStatusAfterLoad("plain-secret")
	if done {
		t.Fatalf("plaintext should not short-circuit, status=%q", status)
	}
}

func TestRunMigrate_AlreadyBcrypt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration in short mode")
	}
	db, connStr := setupMigrateTestDB(t)
	ctx := context.Background()
	hashed := "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
	cfgJSON, _ := json.Marshal(map[string]interface{}{"admin_password": hashed})
	_, err := db.ExecContext(ctx,
		`INSERT INTO admin_config (id, config, updated_at) VALUES ('global', $1, $2) ON CONFLICT (id) DO UPDATE SET config = EXCLUDED.config`,
		string(cfgJSON), time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	status, err := runMigrate(connStr)
	if err != nil {
		t.Fatalf("runMigrate: %v", err)
	}
	if status != "already bcrypt" {
		t.Fatalf("status = %q", status)
	}
}

func TestRunMigrate_MigratesPlaintext(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration in short mode")
	}
	db, connStr := setupMigrateTestDB(t)
	ctx := context.Background()
	cfgJSON, _ := json.Marshal(map[string]interface{}{"admin_password": "plain-secret"})
	_, err := db.ExecContext(ctx,
		`INSERT INTO admin_config (id, config, updated_at) VALUES ('global', $1, $2) ON CONFLICT (id) DO UPDATE SET config = EXCLUDED.config`,
		string(cfgJSON), time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	status, err := runMigrate(connStr)
	if err != nil {
		t.Fatalf("runMigrate: %v", err)
	}
	if status != "migrated" {
		t.Fatalf("status = %q", status)
	}
}
