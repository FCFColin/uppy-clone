//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/testutil"
)

// Integration tests with real databases catch bugs that mocks cannot:
// SQL syntax errors, connection pool exhaustion, constraint violations,
// migration compatibility. testcontainers-go provides disposable, isolated
// database instances for each test run.
// Trade-off: Slower than unit tests (~5s per container), but catches
// real integration issues.

func TestPostgresStore_Integration(t *testing.T) {
	db := testutil.SetupPostgresStore(t)
	ctx := context.Background()

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
