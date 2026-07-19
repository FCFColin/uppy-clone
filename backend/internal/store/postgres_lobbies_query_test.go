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

func TestLoadAllActiveLobbies(t *testing.T) {
	tests := []struct {
		name        string
		limit       int
		cursor      string
		countRow    interface{}
		countErr    error
		fetchErr    error
		fetchRows   *pgxmock.Rows
		wantTotal   int
		wantErr     bool
		skipFetch   bool // true when count fails; fetch expectation should be omitted
		fetchArgs   []interface{}
	}{
		{
			name:      "default limit",
			limit:     0,
			countRow:  0,
			fetchRows: pgxmock.NewRows([]string{"id", "code", "state", "updated_at", "created_at"}),
			fetchArgs: []interface{}{51},
		},
		{
			name:      "capped limit",
			limit:     500,
			countRow:  5,
			fetchRows: pgxmock.NewRows([]string{"id", "code", "state", "updated_at", "created_at"}),
			fetchArgs: []interface{}{101},
		},
		{
			name:      "with cursor",
			limit:     5,
			cursor:    "100|CODE1",
			countRow:  10,
			fetchRows: pgxmock.NewRows([]string{"id", "code", "state", "updated_at", "created_at"}).AddRow("id1", "CODE1", "waiting", int64(99), int64(50)),
			fetchArgs: []interface{}{int64(100), "CODE1", 6},
			wantTotal: 10,
		},
		{
			name:      "count error",
			limit:     10,
			countErr:  errors.New("count failed"),
			wantErr:   true,
			skipFetch: true,
		},
		{
			name:      "fetch error",
			limit:     10,
			countRow:  5,
			fetchErr:  errors.New("query failed"),
			wantErr:   true,
			fetchArgs: []interface{}{11},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockRepo(t, NewLobbyRepository)
			ctx := context.Background()

			countQ := mock.ExpectQuery("SELECT COALESCE\\(reltuples, 0\\)::int FROM pg_class WHERE relname = 'lobby_states'")
			if tt.countErr != nil {
				countQ.WillReturnError(tt.countErr)
			} else {
				countQ.WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(tt.countRow))
			}

			if !tt.skipFetch {
				fetchQ := mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states")
				if len(tt.fetchArgs) > 0 {
					fetchQ.WithArgs(tt.fetchArgs...)
				}
				if tt.fetchErr != nil {
					fetchQ.WillReturnError(tt.fetchErr)
				} else {
					fetchQ.WillReturnRows(tt.fetchRows)
				}
			}

			result, err := repo.LoadAllActiveLobbies(ctx, tt.limit, tt.cursor)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadAllActiveLobbies: %v", err)
			}
			if result == nil {
				t.Fatal("expected result")
			}
			if tt.wantTotal > 0 && result.Total != tt.wantTotal {
				t.Fatalf("total = %d, want %d", result.Total, tt.wantTotal)
			}
		})
	}
}
