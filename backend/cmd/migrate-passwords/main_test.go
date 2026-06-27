package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestIsBcryptHash(t *testing.T) {
	valid := "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
	if !isBcryptHash(valid) {
		t.Fatal("expected valid bcrypt hash")
	}
	tests := []string{"", "plaintext", "$2a$10$short", "x" + valid}
	for _, s := range tests {
		if isBcryptHash(s) {
			t.Fatalf("isBcryptHash(%q) should be false", s)
		}
	}
}

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
	if testing.Short() {
		t.Skip("skipping integration in short mode")
	}
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
	if !ok || !isBcryptHash(hashed) {
		t.Fatalf("expected bcrypt hash in config, got %v", parsed["admin_password"])
	}
}

func setupMigrateTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	var db *sql.DB
	var connStr string
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Skipf("postgres unavailable: %v", r)
			}
		}()
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
			t.Skipf("postgres unavailable: %v", err)
		}
		t.Cleanup(func() { _ = pgContainer.Terminate(ctx) })

		connStr, err = pgContainer.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			t.Fatalf("conn string: %v", err)
		}
		db, err = sql.Open("postgres", connStr)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })

		_, err = db.ExecContext(ctx, `
			CREATE TABLE IF NOT EXISTS admin_config (
				id TEXT PRIMARY KEY,
				config JSONB NOT NULL,
				updated_at BIGINT NOT NULL
			)`)
		if err != nil {
			t.Fatalf("create table: %v", err)
		}
	}()
	return db, connStr
}
