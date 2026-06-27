package main

import (
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/testutil"
)

const testEncryptionKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func setupBackfillPostgres(t *testing.T) (*pgx.Conn, string) {
	t.Helper()
	if err := crypto.Init(testEncryptionKey); err != nil {
		t.Fatalf("crypto.Init: %v", err)
	}

	conn, connStr := testutil.SetupPostgresConn(t)
	testutil.RunMigrationsPGX(t, conn, testutil.BackendMigrationsDir(t), "000009")
	return conn, connStr
}
