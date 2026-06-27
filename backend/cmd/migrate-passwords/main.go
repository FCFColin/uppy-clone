// Command migrate-passwords is a one-time migration script that converts any
// legacy plaintext admin passwords stored in admin_config to bcrypt hashes.
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
func isBcryptHash(s string) bool {
	return len(s) == 60 && (s[:4] == "$2a$" || s[:4] == "$2b$" || s[:4] == "$2y$")
}

// migrationStatusAfterLoad reports whether migration can be skipped after loading config.
func migrationStatusAfterLoad(adminPwd string) (status string, done bool) {
	if isBcryptHash(adminPwd) {
		return "already bcrypt", true
	}
	return "", false
}

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}
	status, err := runMigrate(databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(status)
}

func runMigrate(databaseURL string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := connectDB(ctx, databaseURL)
	if err != nil {
		return "", err
	}
	defer func() { _ = db.Close() }()

	storedConfig, adminPwd, err := loadStoredConfig(ctx, db)
	if err != nil {
		return "", err
	}
	if status, done := migrationStatusAfterLoad(adminPwd); done {
		return status, nil
	}
	if err := migratePasswords(ctx, db, storedConfig, adminPwd); err != nil {
		return "", err
	}
	return "migrated", nil
}

func connectDB(ctx context.Context, databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	return db, nil
}

func loadStoredConfig(ctx context.Context, db *sql.DB) (map[string]interface{}, string, error) {
	var configJSON string
	err := db.QueryRowContext(ctx,
		`SELECT config FROM admin_config WHERE id = 'global'`).
		Scan(&configJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, "", fmt.Errorf("no admin_config row with id='global' found — nothing to migrate")
		}
		return nil, "", fmt.Errorf("failed to query admin_config: %w", err)
	}

	var storedConfig map[string]interface{}
	if err = json.Unmarshal([]byte(configJSON), &storedConfig); err != nil {
		return nil, "", fmt.Errorf("failed to parse config JSON: %w", err)
	}

	adminPwd, ok := storedConfig["admin_password"].(string)
	if !ok || adminPwd == "" {
		return nil, "", fmt.Errorf("admin_password field missing or empty in config")
	}
	return storedConfig, adminPwd, nil
}

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
		`UPDATE admin_config SET config = $1, updated_at = $2 WHERE id = 'global'`,
		string(updatedJSON), time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("failed to update admin_config: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("no rows updated — admin_config row with id='global' not found")
	}
	return nil
}
