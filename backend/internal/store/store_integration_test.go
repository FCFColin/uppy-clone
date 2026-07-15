//go:build integration

package store_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/testutil"
)

// Integration tests for store operations with real PostgreSQL via testcontainers.

func TestConcurrentUserCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := testutil.SetupPostgres(t, testutil.WithStore(), testutil.WithMigrations()).Store
	userRepo := store.NewUserRepository(db.Pool())
	ctx := context.Background()

	email := fmt.Sprintf("concurrent-%d@example.com", time.Now().UnixNano())
	userID := uuid.NewString()

	user := &domain.User{
		ID:        userID,
		Email:     email,
		Nickname:  "ConcurrentUser",
		Palette:   0,
		CreatedAt: time.Now().UnixMilli(),
	}

	// First creation should succeed.
	if err := userRepo.CreateUser(ctx, user); err != nil {
		t.Fatalf("first CreateUser failed: %v", err)
	}

	// Verify user exists.
	got, err := userRepo.GetUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got == nil || got.Email != email {
		t.Fatalf("expected user with email %q, got %+v", email, got)
	}

	// Concurrent duplicate creation should return ErrDuplicateUser.
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dup := &domain.User{
				ID:        uuid.NewString(),
				Email:     email,
				Nickname:  fmt.Sprintf("Dup-%d", time.Now().UnixNano()),
				Palette:   0,
				CreatedAt: time.Now().UnixMilli(),
			}
			if err := userRepo.CreateUser(ctx, dup); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	dupCount := 0
	for err := range errs {
		if err == domain.ErrDuplicateUser {
			dupCount++
		} else {
			t.Errorf("unexpected error on duplicate creation: %v", err)
		}
	}

	if dupCount == 0 {
		t.Fatal("expected at least one ErrDuplicateUser for concurrent duplicate creation")
	}
}

func TestLeaderboardPagination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := testutil.SetupPostgres(t, testutil.WithStore(), testutil.WithMigrations()).Store
	resultRepo := store.NewResultRepository(db.Pool())
	ctx := context.Background()

	now := time.Now().UnixMilli()
	numSessions := 25

	// Insert multiple game sessions.
	for i := 0; i < numSessions; i++ {
		sessionID := uuid.NewString()
		endedAt := now - int64(i)*1000
		if err := resultRepo.CreateGameSession(ctx, &domain.GameSession{
			ID:         sessionID,
			LobbyCode:  fmt.Sprintf("LBRD%d", i),
			Status:     "ended",
			EndedAt:    &endedAt,
			FinalScore: (numSessions - i) * 10,
		}); err != nil {
			t.Fatalf("CreateGameSession %d: %v", i, err)
		}
	}

	// Query all leaderboard entries (max 100 by default).
	entries, err := resultRepo.GetLeaderboard(ctx, "global", 100)
	if err != nil {
		t.Fatalf("GetLeaderboard: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one leaderboard entry")
	}

	// Verify entries are sorted by score descending.
	for i := 1; i < len(entries); i++ {
		if entries[i-1].Score < entries[i].Score {
			t.Fatalf("leaderboard not sorted: entries[%d].Score=%d < entries[%d].Score=%d",
				i-1, entries[i-1].Score, i, entries[i].Score)
		}
	}

	// Verify first entry has highest score.
	if entries[0].Score != numSessions*10 {
		t.Fatalf("expected top score %d, got %d", numSessions*10, entries[0].Score)
	}

	// Query with smaller limit.
	limited, err := resultRepo.GetLeaderboard(ctx, "global", 5)
	if err != nil {
		t.Fatalf("GetLeaderboard limit: %v", err)
	}
	if len(limited) > 5 {
		t.Fatalf("expected at most 5 entries, got %d", len(limited))
	}

	// Query weekly leaderboard.
	weekly, err := resultRepo.GetLeaderboard(ctx, "weekly", 10)
	if err != nil {
		t.Fatalf("GetLeaderboard weekly: %v", err)
	}
	// All sessions were created within the last week, so weekly should return them.
	if len(weekly) == 0 {
		t.Fatal("expected weekly leaderboard entries")
	}
}

func TestGameSessionLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := testutil.SetupPostgres(t, testutil.WithStore(), testutil.WithMigrations()).Store
	resultRepo := store.NewResultRepository(db.Pool())
	ctx := context.Background()

	sessionID := uuid.NewString()
	lobbyCode := "LIFEC"

	// Create a game session in 'active' status.
	startedAt := time.Now().UnixMilli()
	if err := resultRepo.CreateGameSession(ctx, &domain.GameSession{
		ID:        sessionID,
		LobbyCode: lobbyCode,
		Status:    "active",
		StartedAt: &startedAt,
	}); err != nil {
		t.Fatalf("CreateGameSession: %v", err)
	}

	// End the game session and record results.
	endedAt := time.Now().UnixMilli()
	results := []domain.GameResult{
		{
			ID:                uuid.NewString(),
			SessionID:         sessionID,
			UserID:            uuid.NewString(),
			ScoreContribution: 100,
			TapsCount:         10,
			CreatedAt:         endedAt,
		},
		{
			ID:                uuid.NewString(),
			SessionID:         sessionID,
			UserID:            uuid.NewString(),
			ScoreContribution: 75,
			TapsCount:         8,
			CreatedAt:         endedAt,
		},
	}
	if err := resultRepo.EndGameAndRecordResults(ctx, sessionID, endedAt, 175, results); err != nil {
		t.Fatalf("EndGameAndRecordResults: %v", err)
	}
}
