// Command backfill-emails encrypts legacy plaintext user emails and populates email_hash.
//
// Usage:
//
//	DATABASE_URL=... ENCRYPTION_KEY=... go run ./cmd/backfill-emails
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/uppy-clone/backend/internal/crypto"
)

// encryptEmailFn is overridden in tests to simulate per-user encrypt failures.
var encryptEmailFn = crypto.EncryptEmailForStorage

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if err := crypto.InitFromEnv(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	rows, err := conn.Query(ctx, `SELECT id, email FROM users WHERE email_hash IS NULL AND email <> ''`)
	if err != nil {
		return fmt.Errorf("query users: %w", err)
	}

	type pendingUser struct {
		id, email string
	}
	var pending []pendingUser
	for rows.Next() {
		var id, email string
		if err := rows.Scan(&id, &email); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, pendingUser{id: id, email: email})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	var updated int
	for _, user := range pending {
		enc, err := encryptEmailFn(user.email)
		if err != nil {
			return fmt.Errorf("encrypt user %s: %w", user.id, err)
		}
		hash := crypto.EmailHMAC(user.email)
		tag, err := conn.Exec(ctx,
			`UPDATE users SET email = $1, email_hash = $2 WHERE id = $3 AND email_hash IS NULL`,
			enc, hash, user.id)
		if err != nil {
			return fmt.Errorf("update user %s: %w", user.id, err)
		}
		updated += int(tag.RowsAffected())
	}
	fmt.Printf("backfilled %d users\n", updated)
	return nil
}
