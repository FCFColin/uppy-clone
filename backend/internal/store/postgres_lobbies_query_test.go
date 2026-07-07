package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/domain"
)

func TestBuildLobbyListResult_NoMore(t *testing.T) {
	t.Parallel()

	lobbies := []domain.LobbyState{
		{Code: "A", UpdatedAt: 3},
		{Code: "B", UpdatedAt: 2},
	}
	result := buildLobbyListResult(lobbies, 2, 10)
	if result.HasMore {
		t.Fatal("HasMore should be false")
	}
	if result.NextCursor != "" {
		t.Errorf("NextCursor = %q, want empty", result.NextCursor)
	}
	if len(result.Lobbies) != 2 || result.Total != 2 {
		t.Errorf("result = %+v", result)
	}
}

func TestBuildLobbyListResult_Empty(t *testing.T) {
	t.Parallel()

	result := buildLobbyListResult(nil, 0, 20)
	if result.HasMore || result.NextCursor != "" || len(result.Lobbies) != 0 {
		t.Errorf("result = %+v", result)
	}
}

func TestLeaderboardQuery_Scopes(t *testing.T) {
	t.Parallel()

	t.Run("all-time", func(t *testing.T) {
		query, args := leaderboardQuery("all", 25)
		if query == "" || len(args) != 1 || args[0] != 25 {
			t.Errorf("query=%q args=%v", query, args)
		}
	})

	t.Run("weekly", func(t *testing.T) {
		before := time.Now().Add(-7 * 24 * time.Hour).UnixMilli()
		query, args := leaderboardQuery("weekly", 10)
		if query == "" || len(args) != 2 {
			t.Fatalf("query=%q args=%v", query, args)
		}
		cutoff, ok := args[0].(int64)
		if !ok {
			t.Fatalf("cutoff type = %T", args[0])
		}
		if cutoff < before-1000 || cutoff > time.Now().UnixMilli() {
			t.Errorf("cutoff = %d, expected near %d", cutoff, before)
		}
		if args[1] != 10 {
			t.Errorf("limit arg = %v, want 10", args[1])
		}
	})
}

func TestLoadAllActiveLobbies_DefaultLimit(t *testing.T) {
	repo, mock := newMockLobbyRepository(t)
	ctx := context.Background()

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM lobby_states").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states").
		WithArgs(51).
		WillReturnRows(pgxmock.NewRows([]string{"id", "code", "state", "updated_at", "created_at"}))

	result, err := repo.LoadAllActiveLobbies(ctx, 0, "")
	if err != nil {
		t.Fatalf("LoadAllActiveLobbies: %v", err)
	}
	if result == nil || result.Total != 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestLoadAllActiveLobbies_CappedLimit(t *testing.T) {
	repo, mock := newMockLobbyRepository(t)
	ctx := context.Background()

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM lobby_states").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states").
		WithArgs(101).
		WillReturnRows(pgxmock.NewRows([]string{"id", "code", "state", "updated_at", "created_at"}))

	result, err := repo.LoadAllActiveLobbies(ctx, 500, "")
	if err != nil {
		t.Fatalf("LoadAllActiveLobbies: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
}

func TestLoadAllActiveLobbies_WithCursor(t *testing.T) {
	repo, mock := newMockLobbyRepository(t)
	ctx := context.Background()

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM lobby_states").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(10))
	mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states").
		WithArgs(int64(100), "CODE1", 6).
		WillReturnRows(pgxmock.NewRows([]string{"id", "code", "state", "updated_at", "created_at"}).
			AddRow("id1", "CODE1", "waiting", int64(99), int64(50)))

	result, err := repo.LoadAllActiveLobbies(ctx, 5, "100|CODE1")
	if err != nil {
		t.Fatalf("LoadAllActiveLobbies: %v", err)
	}
	if result == nil || result.Total != 10 {
		t.Fatalf("result = %+v", result)
	}
}

func TestLoadAllActiveLobbies_CountError(t *testing.T) {
	repo, mock := newMockLobbyRepository(t)
	ctx := context.Background()

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM lobby_states").
		WillReturnError(errors.New("count failed"))

	_, err := repo.LoadAllActiveLobbies(ctx, 10, "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadAllActiveLobbies_FetchError(t *testing.T) {
	repo, mock := newMockLobbyRepository(t)
	ctx := context.Background()

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM lobby_states").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states").
		WillReturnError(errors.New("query failed"))

	_, err := repo.LoadAllActiveLobbies(ctx, 10, "")
	if err == nil {
		t.Fatal("expected error")
	}
}