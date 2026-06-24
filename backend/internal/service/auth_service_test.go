package service

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/store"
)

// ─── Pure unit tests (no Docker required) ────────────────────────────

// TestNewAuthService verifies the constructor returns non-nil.
func TestNewAuthService(t *testing.T) {
	svc := NewAuthService(nil, nil, nil, nil)
	if svc == nil {
		t.Fatal("NewAuthService returned nil")
	}
}

// ─── Integration test helpers ────────────────────────────────────────

// setupPostgresStore starts a PostgreSQL testcontainer and returns a connected store.
// Skips the test if Docker is unavailable or in short mode.
func setupPostgresStore(t *testing.T) *store.PostgresStore {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

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
		t.Skipf("skipping: postgres container unavailable (Docker not running?): %v", err)
	}
	t.Cleanup(func() { _ = pgContainer.Terminate(ctx) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	timeouts := config.TimeoutConfig{
		PGConnectTimeout: 10 * time.Second,
		PGQueryTimeout:   10 * time.Second,
		PGRequestTimeout: 30 * time.Second,
	}

	db, err := store.NewPostgresStore(connStr, timeouts)
	if err != nil {
		t.Fatalf("failed to create PostgresStore: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	migrationsPath := serviceMigrationsDir(t)
	if err := db.RunMigrations(migrationsPath); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	return db
}

// setupRedisStoreAndManager starts a Redis testcontainer and returns a RedisStore
// and a RefreshTokenManager backed by the same Redis instance.
func setupRedisStoreAndManager(t *testing.T) (*store.RedisStore, *auth.RefreshTokenManager) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	redisContainer, err := tcredis.Run(ctx,
		"redis:7-alpine",
		testcontainers.WithWaitStrategy(
			wait.ForLog("Ready to accept connections").
				WithStartupTimeout(10*time.Second)),
	)
	if err != nil {
		t.Skipf("skipping: redis container unavailable (Docker not running?): %v", err)
	}
	t.Cleanup(func() { _ = redisContainer.Terminate(ctx) })

	addr, err := redisContainer.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("failed to get redis endpoint: %v", err)
	}

	timeouts := config.TimeoutConfig{
		RedisConnectTimeout: 5 * time.Second,
		RedisReadTimeout:    3 * time.Second,
		RedisWriteTimeout:   3 * time.Second,
	}

	redisStore, err := store.NewRedisStore(addr, timeouts)
	if err != nil {
		t.Fatalf("failed to create RedisStore: %v", err)
	}
	t.Cleanup(func() { _ = redisStore.Close() })

	refreshMgr := auth.NewRefreshTokenManager(redisStore.Client())

	return redisStore, refreshMgr
}

// serviceMigrationsDir resolves the absolute path to the backend/migrations directory.
func serviceMigrationsDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	// This file is at backend/internal/service/auth_service_test.go
	// migrations are at backend/migrations/
	dir := filepath.Join(filepath.Dir(filename), "..", "..", "migrations")
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("resolve migrations path: %v", err)
	}
	return abs
}

// createTestUser creates a user in the DB and returns the user ID.
func createTestUser(t *testing.T, db *store.PostgresStore) string {
	t.Helper()
	ctx := context.Background()
	userID := uuid.NewString()
	user := &domain.User{
		ID:        userID,
		Email:     fmt.Sprintf("test-%s@example.com", userID[:8]),
		Nickname:  "TestUser",
		Palette:   0,
		CreatedAt: time.Now().UnixMilli(),
	}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return userID
}

// getUserEmail returns the user's email from the DB.
func getUserEmail(t *testing.T, db *store.PostgresStore, userID string) string {
	t.Helper()
	ctx := context.Background()
	user, err := db.GetUserByID(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if user == nil {
		t.Fatal("user not found")
	}
	return user.Email
}

// getUserNickname returns the user's nickname from the DB.
func getUserNickname(t *testing.T, db *store.PostgresStore, userID string) string {
	t.Helper()
	ctx := context.Background()
	user, err := db.GetUserByID(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if user == nil {
		t.Fatal("user not found")
	}
	return user.Nickname
}

// ─── Integration tests for DeleteUserData ────────────────────────────

// TestDeleteUserData_WithRedis verifies that DeleteUserData revokes all refresh
// tokens and anonymizes the user when Redis is available.
func TestDeleteUserData_WithRedis(t *testing.T) {
	db := setupPostgresStore(t)
	redisStore, refreshMgr := setupRedisStoreAndManager(t)

	userID := createTestUser(t, db)
	ctx := context.Background()

	// Generate refresh tokens for the user
	token1, err := refreshMgr.Generate(ctx, userID)
	if err != nil {
		t.Fatalf("Generate token1: %v", err)
	}
	token2, err := refreshMgr.Generate(ctx, userID)
	if err != nil {
		t.Fatalf("Generate token2: %v", err)
	}

	// Verify tokens are valid before deletion
	if _, err := refreshMgr.Validate(ctx, token1); err != nil {
		t.Fatalf("token1 should be valid before deletion: %v", err)
	}
	if _, err := refreshMgr.Validate(ctx, token2); err != nil {
		t.Fatalf("token2 should be valid before deletion: %v", err)
	}

	// Create AuthService with Redis enabled
	svc := &AuthService{
		db:         db,
		redis:      redisStore,
		jwtMgr:     nil,
		refreshMgr: refreshMgr,
	}

	originalEmail := getUserEmail(t, db, userID)
	originalNickname := getUserNickname(t, db, userID)

	// Delete user data
	if err := svc.DeleteUserData(ctx, userID); err != nil {
		t.Fatalf("DeleteUserData: %v", err)
	}

	// Verify refresh tokens are revoked
	if _, err := refreshMgr.Validate(ctx, token1); err == nil {
		t.Error("token1 should be invalid after DeleteUserData")
	}
	if _, err := refreshMgr.Validate(ctx, token2); err == nil {
		t.Error("token2 should be invalid after DeleteUserData")
	}

	// Verify user is anonymized
	email := getUserEmail(t, db, userID)
	expectedEmail := "deleted_" + userID + "@anonymized"
	if email != expectedEmail {
		t.Errorf("email = %s, want %s", email, expectedEmail)
	}

	nickname := getUserNickname(t, db, userID)
	if nickname != "Deleted User" {
		t.Errorf("nickname = %s, want 'Deleted User'", nickname)
	}

	// Verify original data is gone
	if email == originalEmail {
		t.Error("email should be anonymized, not the original")
	}
	if nickname == originalNickname {
		t.Error("nickname should be anonymized, not the original")
	}
}

// TestDeleteUserData_RedisNil verifies that DeleteUserData still anonymizes the
// user when Redis is nil (the refresh token revocation is skipped).
func TestDeleteUserData_RedisNil(t *testing.T) {
	db := setupPostgresStore(t)

	userID := createTestUser(t, db)
	ctx := context.Background()

	originalEmail := getUserEmail(t, db, userID)
	originalNickname := getUserNickname(t, db, userID)

	// Create AuthService with Redis = nil
	svc := &AuthService{
		db:         db,
		redis:      nil,
		jwtMgr:     nil,
		refreshMgr: nil,
	}

	// Delete user data — should still anonymize even without Redis
	if err := svc.DeleteUserData(ctx, userID); err != nil {
		t.Fatalf("DeleteUserData with redis=nil: %v", err)
	}

	// Verify user is anonymized
	email := getUserEmail(t, db, userID)
	expectedEmail := "deleted_" + userID + "@anonymized"
	if email != expectedEmail {
		t.Errorf("email = %s, want %s (should be anonymized even without Redis)", email, expectedEmail)
	}

	nickname := getUserNickname(t, db, userID)
	if nickname != "Deleted User" {
		t.Errorf("nickname = %s, want 'Deleted User'", nickname)
	}

	// Verify original data is gone
	if email == originalEmail {
		t.Error("email should be anonymized, not the original")
	}
	if nickname == originalNickname {
		t.Error("nickname should be anonymized, not the original")
	}
}

// TestDeleteUserData_DatabaseFailure verifies that DeleteUserData returns an error
// when the database fails (e.g., connection closed).
func TestDeleteUserData_DatabaseFailure(t *testing.T) {
	db := setupPostgresStore(t)
	redisStore, refreshMgr := setupRedisStoreAndManager(t)

	userID := createTestUser(t, db)

	// Close the DB pool to simulate database failure
	db.Close()

	svc := &AuthService{
		db:         db,
		redis:      redisStore,
		jwtMgr:     nil,
		refreshMgr: refreshMgr,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// DeleteUserData should return an error (AnonymizeUser fails)
	err := svc.DeleteUserData(ctx, userID)
	if err == nil {
		t.Fatal("DeleteUserData should return error when DB fails")
	}

	// Note: RevokeAllForUser is called first (and may succeed since Redis is still up),
	// but AnonymizeUser should fail because the DB pool is closed.
	// The error from AnonymizeUser is returned.
}

// TestDeleteUserData_NonExistentUser verifies that DeleteUserData does not fail
// when the user doesn't exist (UPDATE affects 0 rows, which is not an error).
func TestDeleteUserData_NonExistentUser(t *testing.T) {
	db := setupPostgresStore(t)

	nonExistentUserID := uuid.NewString()
	ctx := context.Background()

	svc := &AuthService{
		db:         db,
		redis:      nil,
		jwtMgr:     nil,
		refreshMgr: nil,
	}

	// Should not fail — UPDATE on non-existent user affects 0 rows (not an error)
	err := svc.DeleteUserData(ctx, nonExistentUserID)
	if err != nil {
		t.Errorf("DeleteUserData on non-existent user should not fail: %v", err)
	}
}

// ─── Integration tests for ExportUserData ────────────────────────────

// TestExportUserData_Success verifies that ExportUserData returns the user's data.
func TestExportUserData_Success(t *testing.T) {
	db := setupPostgresStore(t)

	userID := createTestUser(t, db)
	ctx := context.Background()

	svc := &AuthService{
		db:         db,
		redis:      nil,
		jwtMgr:     nil,
		refreshMgr: nil,
	}

	user, err := svc.ExportUserData(ctx, userID)
	if err != nil {
		t.Fatalf("ExportUserData: %v", err)
	}
	if user == nil {
		t.Fatal("ExportUserData returned nil user")
	}
	if user.ID != userID {
		t.Errorf("user.ID = %s, want %s", user.ID, userID)
	}
	if user.Email == "" {
		t.Error("user.Email should not be empty")
	}
	if user.Nickname == "" {
		t.Error("user.Nickname should not be empty")
	}
}

// TestExportUserData_NotFound verifies that ExportUserData returns (nil, nil)
// for a non-existent user.
func TestExportUserData_NotFound(t *testing.T) {
	db := setupPostgresStore(t)

	nonExistentUserID := uuid.NewString()
	ctx := context.Background()

	svc := &AuthService{
		db:         db,
		redis:      nil,
		jwtMgr:     nil,
		refreshMgr: nil,
	}

	user, err := svc.ExportUserData(ctx, nonExistentUserID)
	if err != nil {
		t.Errorf("ExportUserData on non-existent user should not return error: %v", err)
	}
	if user != nil {
		t.Errorf("ExportUserData on non-existent user should return nil, got %+v", user)
	}
}

// TestExportUserData_AfterDeletion verifies that ExportUserData returns the
// anonymized user data after DeleteUserData has been called.
func TestExportUserData_AfterDeletion(t *testing.T) {
	db := setupPostgresStore(t)

	userID := createTestUser(t, db)
	ctx := context.Background()

	svc := &AuthService{
		db:         db,
		redis:      nil,
		jwtMgr:     nil,
		refreshMgr: nil,
	}

	// Delete user data first
	if err := svc.DeleteUserData(ctx, userID); err != nil {
		t.Fatalf("DeleteUserData: %v", err)
	}

	// Export should return anonymized data
	user, err := svc.ExportUserData(ctx, userID)
	if err != nil {
		t.Fatalf("ExportUserData after deletion: %v", err)
	}
	if user == nil {
		t.Fatal("ExportUserData returned nil user after deletion")
	}
	expectedEmail := "deleted_" + userID + "@anonymized"
	if user.Email != expectedEmail {
		t.Errorf("email = %s, want %s", user.Email, expectedEmail)
	}
	if user.Nickname != "Deleted User" {
		t.Errorf("nickname = %s, want 'Deleted User'", user.Nickname)
	}
}

// TestDeleteUserData_FullFlow verifies the complete GDPR deletion flow:
// 1. User has refresh tokens
// 2. DeleteUserData is called
// 3. Tokens are revoked
// 4. User PII is anonymized
// 5. ExportUserData returns anonymized data
func TestDeleteUserData_FullFlow(t *testing.T) {
	db := setupPostgresStore(t)
	redisStore, refreshMgr := setupRedisStoreAndManager(t)

	userID := createTestUser(t, db)
	ctx := context.Background()

	// Generate a refresh token
	token, err := refreshMgr.Generate(ctx, userID)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Verify token is valid
	if _, err := refreshMgr.Validate(ctx, token); err != nil {
		t.Fatalf("token should be valid: %v", err)
	}

	svc := &AuthService{
		db:         db,
		redis:      redisStore,
		jwtMgr:     nil,
		refreshMgr: refreshMgr,
	}

	// Step 1: Export original data
	originalUser, err := svc.ExportUserData(ctx, userID)
	if err != nil || originalUser == nil {
		t.Fatalf("ExportUserData original: err=%v user=%v", err, originalUser)
	}
	originalEmail := originalUser.Email

	// Step 2: Delete user data
	if err := svc.DeleteUserData(ctx, userID); err != nil {
		t.Fatalf("DeleteUserData: %v", err)
	}

	// Step 3: Verify token is revoked
	if _, err := refreshMgr.Validate(ctx, token); err == nil {
		t.Error("token should be invalid after DeleteUserData")
	}

	// Step 4: Verify user is anonymized
	anonymizedUser, err := svc.ExportUserData(ctx, userID)
	if err != nil {
		t.Fatalf("ExportUserData after deletion: %v", err)
	}
	if anonymizedUser == nil {
		t.Fatal("user should still exist (soft delete)")
	}
	if anonymizedUser.Email == originalEmail {
		t.Error("email should be anonymized")
	}
	if anonymizedUser.Email != "deleted_"+userID+"@anonymized" {
		t.Errorf("email = %s, want deleted_%s@anonymized", anonymizedUser.Email, userID)
	}
	if anonymizedUser.Nickname != "Deleted User" {
		t.Errorf("nickname = %s, want 'Deleted User'", anonymizedUser.Nickname)
	}
}
