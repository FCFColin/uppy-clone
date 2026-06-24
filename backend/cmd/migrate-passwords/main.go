// Command migrate-passwords is a one-time migration script that converts any
// legacy plaintext admin passwords stored in app_config to bcrypt hashes.
//
// 企业为何需要：移除明文密码回退分支后，历史明文密码必须迁移为 bcrypt 哈希，
// 否则管理员将无法登录。此脚本应在部署 P0-2 修复后立即执行一次。
//
// Usage:
//
//	DATABASE_URL=postgres://user:pass@host:5432/db?sslmode=disable go run ./cmd/migrate-passwords
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

// isBcryptHash checks if a string looks like a bcrypt hash.
// Mirrors the logic in backend/internal/handler/admin_password.go.
func isBcryptHash(s string) bool {
	return len(s) == 60 && (s[:4] == "$2a$" || s[:4] == "$2b$" || s[:4] == "$2y$")
}

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := connectDB(ctx, databaseURL)
	if err != nil {
		cancel()
		log.Fatalf("%v", err)
	}
	defer func() { _ = db.Close() }()

	storedConfig, adminPwd, err := loadStoredConfig(ctx, db)
	if err != nil {
		log.Fatalf("%v", err)
	}

	if isBcryptHash(adminPwd) {
		fmt.Println("already bcrypt")
		return
	}

	if err := migratePasswords(ctx, db, storedConfig, adminPwd); err != nil {
		log.Fatalf("%v", err)
	}

	fmt.Println("migrated")
}

// connectDB opens the PostgreSQL connection and verifies it with a ping.
func connectDB(ctx context.Context, databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}
	return db, nil
}

// loadStoredConfig reads the app_config row and returns the parsed config
// along with the current admin_password value.
func loadStoredConfig(ctx context.Context, db *sql.DB) (map[string]interface{}, string, error) {
	var configJSON string
	err := db.QueryRowContext(ctx,
		`SELECT config FROM app_config WHERE id = 'global'`).
		Scan(&configJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, "", fmt.Errorf("no app_config row with id='global' found — nothing to migrate")
		}
		return nil, "", fmt.Errorf("failed to query app_config: %w", err)
	}

	var storedConfig map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &storedConfig); err != nil {
		return nil, "", fmt.Errorf("failed to parse config JSON: %w", err)
	}

	adminPwd, ok := storedConfig["admin_password"].(string)
	if !ok || adminPwd == "" {
		return nil, "", fmt.Errorf("admin_password field missing or empty in config")
	}
	return storedConfig, adminPwd, nil
}

// migratePasswords bcrypt-hashes the plaintext admin password and persists
// the updated config back to app_config.
func migratePasswords(ctx context.Context, db *sql.DB, storedConfig map[string]interface{}, adminPwd string) error {
	hashed, err := bcrypt.GenerateFromPassword([]byte(adminPwd), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to bcrypt hash password: %w", err)
	}

	storedConfig["admin_password"] = string(hashed)
	updatedJSON, err := json.Marshal(storedConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal updated config: %w", err)
	}

	result, err := db.ExecContext(ctx,
		`UPDATE app_config SET config = $1, updated_at = $2 WHERE id = 'global'`,
		string(updatedJSON), time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("failed to update app_config: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("no rows updated — app_config row with id='global' not found")
	}
	return nil
}
