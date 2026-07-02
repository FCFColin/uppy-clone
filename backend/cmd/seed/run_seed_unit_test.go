package main

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/testsecrets"
)

func initSeedTestCrypto(t *testing.T) {
	t.Helper()
	if err := crypto.Init(testsecrets.TestEncryptionKeyHex); err != nil {
		t.Fatalf("crypto.Init: %v", err)
	}
}

func expectCreateUser(mock pgxmock.PgxPoolIface) {
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO users").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectExec("INSERT INTO outbox_events").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectCommit()
}

func TestSeedUsers_InsertsUsers(t *testing.T) {
	initSeedTestCrypto(t)
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

	for i := 0; i < 3; i++ {
		expectCreateUser(mock)
	}

	db := store.NewPostgresStoreWithPool(mock)
	users := seedUsers(context.Background(), db, 1000)
	if len(users) != 3 {
		t.Fatalf("users = %d, want 3", len(users))
	}
}

func TestSeedSessions_InsertsSessions(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

	for i := 0; i < 5; i++ {
		mock.ExpectExec("INSERT INTO game_sessions").
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	}

	db := store.NewPostgresStoreWithPool(mock)
	ids := seedSessions(context.Background(), db, 2000)
	if len(ids) != 5 {
		t.Fatalf("session ids = %d, want 5", len(ids))
	}
}

func TestSeedResults_InsertsResults(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

	for i := 0; i < 10; i++ {
		mock.ExpectExec("INSERT INTO game_results").
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	}

	db := store.NewPostgresStoreWithPool(mock)
	users := []*domain.User{
		{ID: "u1"}, {ID: "u2"}, {ID: "u3"},
	}
	sessionIDs := []string{"s1", "s2", "s3", "s4", "s5"}
	seedResults(context.Background(), db, 3000, users, sessionIDs)
}

func TestRunSeed_SuccessHooked(t *testing.T) {
	initSeedTestCrypto(t)
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

	for i := 0; i < 3; i++ {
		expectCreateUser(mock)
	}
	for i := 0; i < 5; i++ {
		mock.ExpectExec("INSERT INTO game_sessions").
			WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	}
	for i := 0; i < 10; i++ {
		mock.ExpectExec("INSERT INTO game_results").
			WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	}

	orig := newPostgresStoreFn
	newPostgresStoreFn = func(_ string, _ config.TimeoutConfig) (*store.PostgresStore, error) {
		return store.NewPostgresStoreWithPool(mock), nil
	}
	t.Cleanup(func() { newPostgresStoreFn = orig })

	if err := runSeed("postgres://u:p@127.0.0.1/dev?sslmode=disable"); err != nil {
		t.Fatalf("runSeed: %v", err)
	}
}

func TestMain_SuccessHooked(t *testing.T) {
	initSeedTestCrypto(t)
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

	for i := 0; i < 3; i++ {
		expectCreateUser(mock)
	}
	for i := 0; i < 5; i++ {
		mock.ExpectExec("INSERT INTO game_sessions").
			WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	}
	for i := 0; i < 10; i++ {
		mock.ExpectExec("INSERT INTO game_results").
			WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	}

	orig := newPostgresStoreFn
	newPostgresStoreFn = func(_ string, _ config.TimeoutConfig) (*store.PostgresStore, error) {
		return store.NewPostgresStoreWithPool(mock), nil
	}
	t.Cleanup(func() { newPostgresStoreFn = orig })

	t.Setenv("DATABASE_URL", "postgres://u:p@127.0.0.1/dev?sslmode=disable")
	main()
}
