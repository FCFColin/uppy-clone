package integration

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
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/store"
)

// Integration tests with real databases catch bugs that mocks cannot:
// SQL syntax errors, connection pool exhaustion, constraint violations,
// migration compatibility. testcontainers-go provides disposable, isolated
// database instances for each test run.
// Trade-off: Slower than unit tests (~5s per container), but catches
// real integration issues.

func TestPostgresStore_Integration(t *testing.T) {
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
		t.Fatalf("failed to start postgres container: %v", err)
	}
	defer pgContainer.Terminate(ctx)

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
	defer db.Close()

	// Run migrations
	migrationsPath := migrationsDir(t)
	if err := db.RunMigrations(migrationsPath); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	t.Run("CreateUserAndGetByEmail", func(t *testing.T) {
		email := fmt.Sprintf("test-%d@example.com", time.Now().UnixNano())
		user := &domain.User{
			ID:        uuid.NewString(),
			Email:     email,
			Nickname:  "TestUser",
			Palette:   0,
			CreatedAt: time.Now().UnixMilli(),
		}
		if err := db.CreateUser(ctx, user); err != nil {
			t.Fatalf("CreateUser failed: %v", err)
		}

		got, err := db.GetUserByEmail(ctx, email)
		if err != nil {
			t.Fatalf("GetUserByEmail failed: %v", err)
		}
		if got == nil {
			t.Fatal("GetUserByEmail returned nil")
		}
		if got.Email != email {
			t.Fatalf("expected email %s, got %s", email, got.Email)
		}
		if got.Nickname != "TestUser" {
			t.Fatalf("expected nickname TestUser, got %s", got.Nickname)
		}
	})

	t.Run("SaveAndLoadLobbyState", func(t *testing.T) {
		code := fmt.Sprintf("T%d", time.Now().UnixNano()%100000)
		lobby := &domain.LobbyState{
			ID:        uuid.NewString(),
			Code:      code,
			State:     `{"phase":"waiting"}`,
			UpdatedAt: time.Now().UnixMilli(),
			CreatedAt: time.Now().UnixMilli(),
		}
		if err := db.SaveLobbyState(ctx, lobby); err != nil {
			t.Fatalf("SaveLobbyState failed: %v", err)
		}

		// Verify via LoadLobbyState
		got, err := db.LoadLobbyState(ctx, code)
		if err != nil {
			t.Fatalf("LoadLobbyState failed: %v", err)
		}
		if got == nil {
			t.Fatal("LoadLobbyState returned nil")
		}
		if got.Code != code {
			t.Fatalf("expected code %s, got %s", code, got.Code)
		}

		// Verify via LoadAllActiveLobbies
		result, err := db.LoadAllActiveLobbies(ctx, 10, "")
		if err != nil {
			t.Fatalf("LoadAllActiveLobbies failed: %v", err)
		}
		found := false
		for _, l := range result.Lobbies {
			if l.Code == code {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("saved lobby not found in active lobbies")
		}
	})
}

// migrationsDir resolves the absolute path to the backend/migrations directory.
func migrationsDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	// This file is at backend/tests/integration/postgres_test.go
	// migrations are at backend/migrations/
	dir := filepath.Join(filepath.Dir(filename), "..", "..", "migrations")
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("resolve migrations path: %v", err)
	}
	return abs
}
