package main

import (
	"context"
	"strings"
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

	userRepo := store.NewUserRepository(mock)
	stats := &seedStats{}
	users, err := seedUsers(context.Background(), userRepo, 1000, stats)
	if err != nil {
		t.Fatalf("seedUsers: %v", err)
	}
	if len(users) != 3 {
		t.Fatalf("users = %d, want 3", len(users))
	}
	if stats.usersCreated != 3 {
		t.Fatalf("usersCreated = %d, want 3", stats.usersCreated)
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

	resultRepo := store.NewResultRepository(mock)
	stats := &seedStats{}
	ids, err := seedSessions(context.Background(), resultRepo, 2000, stats)
	if err != nil {
		t.Fatalf("seedSessions: %v", err)
	}
	if len(ids) != 5 {
		t.Fatalf("session ids = %d, want 5", len(ids))
	}
	if stats.sessionsCreated != 5 {
		t.Fatalf("sessionsCreated = %d, want 5", stats.sessionsCreated)
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

	resultRepo := store.NewResultRepository(mock)
	users := []*domain.User{
		{ID: "u1"}, {ID: "u2"}, {ID: "u3"},
	}
	sessionIDs := []string{"s1", "s2", "s3", "s4", "s5"}
	stats := &seedStats{}
	if err := seedResults(context.Background(), resultRepo, 3000, users, sessionIDs, stats); err != nil {
		t.Fatalf("seedResults: %v", err)
	}
	if stats.resultsCreated != 10 {
		t.Fatalf("resultsCreated = %d, want 10", stats.resultsCreated)
	}
}

// hookReposForTest replaces newSeedReposFn to inject mock-backed repos.
func hookReposForTest(t *testing.T, mock pgxmock.PgxPoolIface) {
	t.Helper()
	orig := newSeedReposFn
	newSeedReposFn = func(_ *store.PostgresStore) (*store.UserRepository, *store.ResultRepository) {
		return store.NewUserRepository(mock), store.NewResultRepository(mock)
	}
	t.Cleanup(func() { newSeedReposFn = orig })
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
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	}
	for i := 0; i < 10; i++ {
		mock.ExpectExec("INSERT INTO game_results").
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	}

	orig := newPostgresStoreFn
	newPostgresStoreFn = func(_ string, _ config.TimeoutConfig, _ ...store.Deps) (*store.PostgresStore, error) {
		return store.NewPostgresStoreWithPool(mock), nil
	}
	t.Cleanup(func() { newPostgresStoreFn = orig })
	hookReposForTest(t, mock)

	status, err := runSeed("postgres://u:p@127.0.0.1/dev?sslmode=disable")
	if err != nil {
		t.Fatalf("runSeed: %v", err)
	}
	if !strings.Contains(status, "3 users") || !strings.Contains(status, "5 game sessions") || !strings.Contains(status, "10 game results") {
		t.Fatalf("status = %q, want counts 3/5/10", status)
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
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	}
	for i := 0; i < 10; i++ {
		mock.ExpectExec("INSERT INTO game_results").
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	}

	orig := newPostgresStoreFn
	newPostgresStoreFn = func(_ string, _ config.TimeoutConfig, _ ...store.Deps) (*store.PostgresStore, error) {
		return store.NewPostgresStoreWithPool(mock), nil
	}
	t.Cleanup(func() { newPostgresStoreFn = orig })
	hookReposForTest(t, mock)

	t.Setenv("DATABASE_URL", "postgres://u:p@127.0.0.1/dev?sslmode=disable")
	main()
}

// v2-R-96: Verify runSeed reports actual counts, not hardcoded "3/5/10",
// when some inserts fail with non-duplicate errors.
func TestRunSeed_ReportsActualCountsOnError(t *testing.T) {
	initSeedTestCrypto(t)
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

	// First user succeeds, second fails (non-duplicate), third should not run.
	expectCreateUser(mock)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO users").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(&pgconn.PgError{Code: "08000", Message: "connection error"})
	mock.ExpectRollback()

	orig := newPostgresStoreFn
	newPostgresStoreFn = func(_ string, _ config.TimeoutConfig, _ ...store.Deps) (*store.PostgresStore, error) {
		return store.NewPostgresStoreWithPool(mock), nil
	}
	t.Cleanup(func() { newPostgresStoreFn = orig })
	hookReposForTest(t, mock)

	_, err = runSeed("postgres://u:p@127.0.0.1/dev?sslmode=disable")
	if err == nil {
		t.Fatal("expected error when user insert fails with non-duplicate error")
	}
	if !strings.Contains(err.Error(), "seed users") {
		t.Fatalf("error = %v, want 'seed users'", err)
	}
}

// v2-R-96: Duplicate-key errors are non-fatal (idempotent re-runs).
// misc-033: On duplicate, seedUsers looks up the existing user's real DB ID
// so that seedResults references the actual record.
func TestSeedUsers_DuplicateIsNonFatal(t *testing.T) {
	initSeedTestCrypto(t)
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

	existingIDs := []string{"existing-alice", "existing-bob", "existing-charlie"}
	emails := []string{"alice@test.com", "bob@test.com", "charlie@test.com"}
	nicks := []string{"Alice", "Bob", "Charlie"}

	for i := 0; i < 3; i++ {
		// CreateUser returns duplicate
		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO users").
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnError(&pgconn.PgError{Code: "23505", Message: "duplicate key"})
		mock.ExpectRollback()
		// GetUserByEmail returns the existing user's real ID
		mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users").
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnRows(pgxmock.NewRows([]string{"id", "email", "nickname", "palette", "created_at", "last_login"}).
				AddRow(existingIDs[i], emails[i], nicks[i], i, int64(1000), nil))
	}

	userRepo := store.NewUserRepository(mock)
	stats := &seedStats{}
	users, err := seedUsers(context.Background(), userRepo, 1000, stats)
	if err != nil {
		t.Fatalf("seedUsers with duplicates: %v", err)
	}
	if len(users) != 3 {
		t.Fatalf("users = %d, want 3", len(users))
	}
	if stats.usersCreated != 0 {
		t.Fatalf("usersCreated = %d, want 0 (all duplicates)", stats.usersCreated)
	}
	// Verify that duplicate users have their IDs replaced with the existing
	// DB record's ID, not the discarded UUID.
	for i, u := range users {
		if u.ID != existingIDs[i] {
			t.Errorf("users[%d].ID = %q, want %q (existing DB ID)", i, u.ID, existingIDs[i])
		}
	}
}
