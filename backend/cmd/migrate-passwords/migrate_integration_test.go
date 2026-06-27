//go:build integration

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/testutil"
)

func TestLoadStoredConfig_Success(t *testing.T) {
	db, _ := setupMigrateTestDB(t)
	ctx := context.Background()
	cfgJSON := `{"admin_password":"secret"}`
	_, err := db.ExecContext(ctx,
		`INSERT INTO admin_config (id, config, updated_at) VALUES ('global', $1, $2)`,
		cfgJSON, time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	cfg, pwd, err := loadStoredConfig(ctx, db)
	if err != nil {
		t.Fatalf("loadStoredConfig: %v", err)
	}
	if pwd != "secret" {
		t.Fatalf("password = %q", pwd)
	}
	if cfg == nil {
		t.Fatal("expected config map")
	}
}

func TestLoadStoredConfig_MissingRow(t *testing.T) {
	db, _ := setupMigrateTestDB(t)
	ctx := context.Background()
	_, _, err := loadStoredConfig(ctx, db)
	if err == nil {
		t.Fatal("expected error for missing admin_config row")
	}
}

func TestMigratePasswords_Integration(t *testing.T) {
	db, _ := setupMigrateTestDB(t)
	ctx := context.Background()

	plainPwd := "admin-plain-secret"
	cfg := map[string]interface{}{"admin_password": plainPwd}
	cfgJSON, _ := json.Marshal(cfg)
	_, err := db.ExecContext(ctx,
		`INSERT INTO admin_config (id, config, updated_at) VALUES ('global', $1, $2)`,
		string(cfgJSON), time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("insert admin_config: %v", err)
	}

	if err := migratePasswords(ctx, db, cfg, plainPwd); err != nil {
		t.Fatalf("migratePasswords: %v", err)
	}

	var stored string
	if err := db.QueryRowContext(ctx, `SELECT config FROM admin_config WHERE id = 'global'`).Scan(&stored); err != nil {
		t.Fatalf("query: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(stored), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	hashed, ok := parsed["admin_password"].(string)
	if !ok || !crypto.IsBcryptHash(hashed) {
		t.Fatalf("expected bcrypt hash in config, got %v", parsed["admin_password"])
	}
}

func setupMigrateTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	conn, connStr := testutil.SetupPostgresConn(t)
	testutil.RunMigrationsPGX(t, conn, testutil.BackendMigrationsDir(t), "000009")

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, connStr
}

func TestRunMigrate_AlreadyBcrypt(t *testing.T) {
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
