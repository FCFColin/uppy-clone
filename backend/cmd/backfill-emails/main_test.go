package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/testsecrets"
)

func TestRun_MissingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	err := run()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL missing")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("error = %v", err)
	}
}

func TestRun_MissingEncryptionKey(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@127.0.0.1:1/db?sslmode=disable")
	_ = os.Unsetenv("ENCRYPTION_KEY")
	err := run()
	if err == nil {
		t.Fatal("expected error when ENCRYPTION_KEY missing")
	}
}

func TestRun_ConnectFailure(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://invalid:invalid@127.0.0.1:1/nodb?sslmode=disable")
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
	err := run()
	if err == nil || !strings.Contains(err.Error(), "connect") {
		t.Fatalf("expected connect error, got %v", err)
	}
}

func TestRun_QueryFailure(t *testing.T) {
	mock, err := pgxmock.NewConn()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close(context.Background()) })

	orig := pgxConnectFn
	pgxConnectFn = func(context.Context, string) (backfillDB, error) { return mock, nil }
	t.Cleanup(func() { pgxConnectFn = orig })

	mock.ExpectQuery("SELECT id, email FROM users").
		WillReturnError(fmt.Errorf("query failed"))

	t.Setenv("DATABASE_URL", "postgres://mock/mock?sslmode=disable")
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
	err = run()
	if err == nil || !strings.Contains(err.Error(), "query users") {
		t.Fatalf("expected query error, got %v", err)
	}
}

func TestRun_BackfillsUsers(t *testing.T) {
	if err := crypto.Init(testsecrets.TestEncryptionKeyHex); err != nil {
		t.Fatalf("crypto.Init: %v", err)
	}

	mock, err := pgxmock.NewConn()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close(context.Background()) })

	orig := pgxConnectFn
	pgxConnectFn = func(context.Context, string) (backfillDB, error) { return mock, nil }
	t.Cleanup(func() { pgxConnectFn = orig })

	mock.ExpectQuery("SELECT id, email FROM users").
		WillReturnRows(pgxmock.NewRows([]string{"id", "email"}).
			AddRow("user-1", "plain@example.com"))
	mock.ExpectExec("UPDATE users SET email").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), "user-1").
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))

	t.Setenv("DATABASE_URL", "postgres://mock/mock?sslmode=disable")
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
	if err := run(); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestRun_NoPendingUsers(t *testing.T) {
	mock, err := pgxmock.NewConn()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close(context.Background()) })

	orig := pgxConnectFn
	pgxConnectFn = func(context.Context, string) (backfillDB, error) { return mock, nil }
	t.Cleanup(func() { pgxConnectFn = orig })

	mock.ExpectQuery("SELECT id, email FROM users").
		WillReturnRows(pgxmock.NewRows([]string{"id", "email"}))

	t.Setenv("DATABASE_URL", "postgres://mock/mock?sslmode=disable")
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
	if err := run(); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestMain_Success(t *testing.T) {
	mock, err := pgxmock.NewConn()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close(context.Background()) })

	orig := pgxConnectFn
	pgxConnectFn = func(context.Context, string) (backfillDB, error) { return mock, nil }
	t.Cleanup(func() { pgxConnectFn = orig })

	mock.ExpectQuery("SELECT id, email FROM users").
		WillReturnRows(pgxmock.NewRows([]string{"id", "email"}))

	t.Setenv("DATABASE_URL", "postgres://mock/mock?sslmode=disable")
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
	main()
}
