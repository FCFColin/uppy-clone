//go:build integration

package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/uppy-clone/backend/internal/crypto"
)

func TestBackfillEmails_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration in short mode")
	}

	db, connStr := setupBackfillPostgres(t)
	ctx := context.Background()

	id1 := uuid.NewString()
	id2 := uuid.NewString()
	email1 := fmt.Sprintf("backfill-a-%d@example.com", time.Now().UnixNano())
	email2 := fmt.Sprintf("backfill-b-%d@example.com", time.Now().UnixNano())
	for _, row := range []struct{ id, email string }{
		{id1, email1},
		{id2, email2},
	} {
		_, err := db.Exec(ctx,
			`INSERT INTO users (id, email, nickname, created_at) VALUES ($1::uuid, $2, $3, $4)`,
			row.id, row.email, "test", time.Now().UnixMilli())
		if err != nil {
			t.Fatalf("insert user %s: %v", row.id, err)
		}
	}

	t.Setenv("DATABASE_URL", connStr)
	t.Setenv("ENCRYPTION_KEY", testEncryptionKey)
	if err := run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	for _, row := range []struct{ id, plain string }{
		{id1, email1},
		{id2, email2},
	} {
		var storedEmail, storedHash string
		err := db.QueryRow(ctx,
			`SELECT email, email_hash FROM users WHERE id = $1::uuid`, row.id).
			Scan(&storedEmail, &storedHash)
		if err != nil {
			t.Fatalf("query user %s: %v", row.id, err)
		}
		if !strings.HasPrefix(storedEmail, "v1:") {
			t.Fatalf("user %s email not encrypted, got %q", row.id, storedEmail)
		}
		decrypted, err := crypto.DecryptEmailFromStorage(storedEmail)
		if err != nil || decrypted != row.plain {
			t.Fatalf("decrypt user %s: err=%v got=%q want=%q", row.id, err, decrypted, row.plain)
		}
		wantHash := crypto.EmailHMAC(row.plain)
		if storedHash != wantHash {
			t.Fatalf("user %s hash = %q want %q", row.id, storedHash, wantHash)
		}
	}
}

func TestBackfillEmails_StopOnEncryptError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration in short mode")
	}

	db, connStr := setupBackfillPostgres(t)
	ctx := context.Background()

	badID := uuid.NewString()
	badEmail := fmt.Sprintf("bad-%d@example.com", time.Now().UnixNano())
	_, err := db.Exec(ctx,
		`INSERT INTO users (id, email, nickname, created_at) VALUES ($1::uuid, $2, $3, $4)`,
		badID, badEmail, "test", time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	origEncrypt := encryptEmailFn
	encryptEmailFn = func(string) (string, error) {
		return "", fmt.Errorf("simulated encrypt failure")
	}
	t.Cleanup(func() { encryptEmailFn = origEncrypt })

	t.Setenv("DATABASE_URL", connStr)
	t.Setenv("ENCRYPTION_KEY", testEncryptionKey)
	err = run()
	if err == nil || !strings.Contains(err.Error(), "encrypt user") {
		t.Fatalf("expected encrypt error, got %v", err)
	}

	var storedEmail string
	var storedHash *string
	if err := db.QueryRow(ctx,
		`SELECT email, email_hash FROM users WHERE id = $1::uuid`, badID).
		Scan(&storedEmail, &storedHash); err != nil {
		t.Fatalf("query user: %v", err)
	}
	if storedEmail != badEmail {
		t.Fatalf("email changed to %q, want plaintext preserved", storedEmail)
	}
	if storedHash != nil {
		t.Fatalf("email_hash should remain null, got %v", *storedHash)
	}
}
