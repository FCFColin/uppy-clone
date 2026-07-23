package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/domain"
)

// --- Leaderboard scan & query ---

type mockLeaderboardRows struct {
	mockRowsBase
	scores       []int
	displayNames []string
	endedAts     []int64
}

func (m *mockLeaderboardRows) Next() bool { return m.next(len(m.scores)) }
func (m *mockLeaderboardRows) Scan(dest ...interface{}) error {
	if m.scanErr != nil {
		return m.scanErr
	}
	if m.pos == 0 || m.pos > len(m.scores) {
		return fmt.Errorf("scan out of range")
	}
	i := m.pos - 1
	*dest[0].(*int) = m.scores[i]
	*dest[1].(*string) = m.displayNames[i]
	*dest[2].(*int64) = m.endedAts[i]
	return nil
}

func TestScanLeaderboardRows(t *testing.T) {
	t.Parallel()

	t.Run("assigns ranks", func(t *testing.T) {
		rows := &mockLeaderboardRows{
			scores:       []int{900, 800},
			displayNames: []string{"Alice", "Bob"},
			endedAts:     []int64{100, 200},
		}
		entries, err := scanLeaderboardRows(rows)
		if err != nil {
			t.Fatalf("scanLeaderboardRows: %v", err)
		}
		if len(entries) != 2 {
			t.Fatalf("got %d entries, want 2", len(entries))
		}
		if entries[0].Rank != 1 || entries[0].Score != 900 || entries[0].Name != "Alice" {
			t.Errorf("entry[0] = %+v", entries[0])
		}
		if entries[1].Rank != 2 || entries[1].Score != 800 || entries[1].Name != "Bob" {
			t.Errorf("entry[1] = %+v", entries[1])
		}
	})

	t.Run("scan error propagates", func(t *testing.T) {
		rows := &mockLeaderboardRows{
			scores:       []int{100},
			displayNames: []string{"C"},
			mockRowsBase: mockRowsBase{scanErr: errors.New("field type mismatch")},
		}
		_, err := scanLeaderboardRows(rows)
		if err == nil || !strings.Contains(err.Error(), "scan leaderboard row") {
			t.Errorf("expected scan error, got %v", err)
		}
	})

	t.Run("rows.Err propagates", func(t *testing.T) {
		rows := &mockLeaderboardRows{
			scores:       []int{100},
			displayNames: []string{"D"},
			mockRowsBase: mockRowsBase{err: errors.New("connection lost")},
		}
		_, err := scanLeaderboardRows(rows)
		if err == nil || !strings.Contains(err.Error(), "connection lost") {
			t.Errorf("expected connection error, got %v", err)
		}
	})
}

func TestGetLeaderboard(t *testing.T) {
	tests := []struct {
		name          string
		scope         string
		limit         int
		queryErr      error
		rows          *pgxmock.Rows
		wantEntries   int
		wantFirstName string
		wantErr       bool
	}{
		{
			name:          "success",
			scope:         "all",
			limit:         50,
			rows:          pgxmock.NewRows([]string{"best_score", "display_name", "best_at"}).AddRow(1000, "Alice", int64(100)).AddRow(800, "Bob", int64(200)),
			wantEntries:   2,
			wantFirstName: "Alice",
		},
		{
			name:        "empty returns slice",
			scope:       "all",
			limit:       0, // internally replaced with 50
			rows:        pgxmock.NewRows([]string{"best_score", "display_name", "best_at"}),
			wantEntries: 0,
		},
		{
			// Regression: when nickname is empty, COALESCE falls back to
			// gr.user_id::text. The mock returns a UUID-like string as
			// display_name, simulating the DB-side COALESCE result.
			name:          "no nickname falls back to user_id text",
			scope:         "all",
			limit:         50,
			rows:          pgxmock.NewRows([]string{"best_score", "display_name", "best_at"}).AddRow(700, "550e8400-e29b-41d4-a716-446655440000", int64(150)),
			wantEntries:   1,
			wantFirstName: "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:     "query error",
			scope:    "weekly",
			limit:    10,
			queryErr: errors.New("query failed"),
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockRepo(t, NewResultRepository)
			ctx := context.Background()

			// GetLeaderboard replaces limit<=0 with 50
			effectiveLimit := tt.limit
			if effectiveLimit <= 0 || effectiveLimit > 100 {
				effectiveLimit = 50
			}

			if tt.queryErr != nil {
				mock.ExpectQuery(`SELECT MAX\(gr\.score_contribution\)`).WillReturnError(tt.queryErr)
			} else {
				mock.ExpectQuery(`SELECT MAX\(gr\.score_contribution\)`).WithArgs(effectiveLimit).WillReturnRows(tt.rows)
			}

			entries, err := repo.GetLeaderboard(ctx, tt.scope, tt.limit)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("GetLeaderboard: %v", err)
			}
			if tt.wantEntries == 0 {
				if entries == nil {
					t.Fatal("expected non-nil slice")
				}
			}
			if len(entries) != tt.wantEntries {
				t.Fatalf("got %d entries, want %d", len(entries), tt.wantEntries)
			}
			if tt.wantFirstName != "" && len(entries) > 0 && entries[0].Name != tt.wantFirstName {
				t.Fatalf("entries[0].Name = %q, want %q", entries[0].Name, tt.wantFirstName)
			}
		})
	}
}

func TestGetUserBestScore(t *testing.T) {
	tests := []struct {
		name      string
		queryErr  error
		wantScore int
		wantGames int
		wantErr   bool
	}{
		{"success", nil, 500, 3, false},
		{"query error", errors.New("query failed"), 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockRepo(t, NewResultRepository)
			ctx := context.Background()

			if tt.queryErr != nil {
				mock.ExpectQuery("SELECT COALESCE").WillReturnError(tt.queryErr)
			} else {
				mock.ExpectQuery("SELECT COALESCE").WithArgs("user-1").
					WillReturnRows(pgxmock.NewRows([]string{"max", "count"}).AddRow(tt.wantScore, tt.wantGames))
			}

			score, games, err := repo.GetUserBestScore(ctx, "user-1")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("GetUserBestScore: %v", err)
			}
			if score != tt.wantScore || games != tt.wantGames {
				t.Fatalf("GetUserBestScore = (%d,%d), want (%d,%d)", score, games, tt.wantScore, tt.wantGames)
			}
		})
	}
}

// --- Lobby list query ---

func TestLoadAllActiveLobbies(t *testing.T) {
	tests := []struct {
		name      string
		limit     int
		cursor    string
		countRow  interface{}
		countErr  error
		fetchErr  error
		fetchRows *pgxmock.Rows
		wantTotal int
		wantErr   bool
		skipFetch bool // true when count fails; fetch expectation should be omitted
		fetchArgs []interface{}
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

// --- Lobby state CRUD ---

func TestSaveLobbyState(t *testing.T) {
	tests := []struct {
		name    string
		execErr error
		wantErr bool
	}{
		{"success", nil, false},
		{"error", errors.New("save failed"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockRepo(t, NewLobbyRepository)
			exec := mock.ExpectExec("INSERT INTO lobby_states").
				WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg())
			expectExecResult(exec, tt.execErr, "INSERT 1")
			err := repo.SaveLobbyState(context.Background(), &domain.LobbyState{ID: "l1", Code: "ABCD1", State: "waiting", UpdatedAt: 100, CreatedAt: 50})
			assertWantErr(t, err, tt.wantErr, "SaveLobbyState")
		})
	}
}

func TestLoadLobbyState(t *testing.T) {
	tests := []struct {
		name     string
		queryErr error
		rowErr   error
		rows     *pgxmock.Rows
		wantNil  bool
		wantErr  bool
	}{
		{
			name: "found",
			rows: pgxmock.NewRows([]string{"id", "code", "state", "updated_at", "created_at"}).
				AddRow("l1", "ABCD1", "playing", int64(200), int64(100)),
		},
		{
			name:     "not found",
			queryErr: pgx.ErrNoRows,
			wantNil:  true,
		},
		{
			name:    "scan error",
			rows:    pgxmock.NewRows([]string{"id", "code", "state", "updated_at", "created_at"}).AddRow("l1", "ABCD1", "playing", int64(200), int64(100)),
			rowErr:  errors.New("scan failed"),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockRepo(t, NewLobbyRepository)
			q := mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states WHERE code").
				WithArgs("ABCD1")
			if tt.queryErr != nil {
				q.WillReturnError(tt.queryErr)
			} else {
				if tt.rowErr != nil {
					tt.rows.RowError(0, tt.rowErr)
				}
				q.WillReturnRows(tt.rows)
			}
			ls, err := repo.LoadLobbyState(context.Background(), "ABCD1")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadLobbyState: %v", err)
			}
			if tt.wantNil && ls != nil {
				t.Fatalf("expected nil, got %+v", ls)
			}
			if !tt.wantNil && ls == nil {
				t.Fatal("expected non-nil lobby state")
			}
		})
	}
}

func TestDeleteLobbyState(t *testing.T) {
	tests := []struct {
		name    string
		execErr error
		wantErr bool
	}{
		{"success", nil, false},
		{"error", errors.New("delete failed"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockRepo(t, NewLobbyRepository)
			exec := mock.ExpectExec("DELETE FROM lobby_states WHERE code").
				WithArgs("ABCD1")
			expectExecResult(exec, tt.execErr, "DELETE 1")
			err := repo.DeleteLobbyState(context.Background(), "ABCD1")
			assertWantErr(t, err, tt.wantErr, "DeleteLobbyState")
		})
	}
}
