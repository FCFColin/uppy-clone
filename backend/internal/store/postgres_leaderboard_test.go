package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/pashagolub/pgxmock/v4"
)

type mockLeaderboardRows struct {
	mockRowsBase
	scores   []int
	codes    []string
	endedAts []int64
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
	*dest[1].(*string) = m.codes[i]
	*dest[2].(*int64) = m.endedAts[i]
	return nil
}

func TestScanLeaderboardRows(t *testing.T) {
	t.Parallel()

	t.Run("assigns ranks", func(t *testing.T) {
		rows := &mockLeaderboardRows{
			scores:   []int{900, 800},
			codes:    []string{"A", "B"},
			endedAts: []int64{100, 200},
		}
		entries, err := scanLeaderboardRows(rows)
		if err != nil {
			t.Fatalf("scanLeaderboardRows: %v", err)
		}
		if len(entries) != 2 {
			t.Fatalf("got %d entries, want 2", len(entries))
		}
		if entries[0].Rank != 1 || entries[0].Score != 900 || entries[0].LobbyCode != "A" {
			t.Errorf("entry[0] = %+v", entries[0])
		}
		if entries[1].Rank != 2 || entries[1].Score != 800 || entries[1].LobbyCode != "B" {
			t.Errorf("entry[1] = %+v", entries[1])
		}
	})

	t.Run("scan error propagates", func(t *testing.T) {
		rows := &mockLeaderboardRows{
			scores:       []int{100},
			codes:        []string{"C"},
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
			codes:        []string{"D"},
			mockRowsBase: mockRowsBase{err: errors.New("connection lost")},
		}
		_, err := scanLeaderboardRows(rows)
		if err == nil || !strings.Contains(err.Error(), "connection lost") {
			t.Errorf("expected connection error, got %v", err)
		}
	})

	t.Run("single row", func(t *testing.T) {
		rows := &mockLeaderboardRows{
			scores:   []int{500},
			codes:    []string{"X"},
			endedAts: []int64{300},
		}
		entries, err := scanLeaderboardRows(rows)
		if err != nil {
			t.Fatalf("scanLeaderboardRows: %v", err)
		}
		if len(entries) != 1 || entries[0].Score != 500 || entries[0].Rank != 1 {
			t.Errorf("entries = %+v", entries)
		}
	})
}

func TestLeaderboardQuery(t *testing.T) {
	t.Run("global scope", func(t *testing.T) {
		query, args := leaderboardQuery("global", 10)
		if len(args) != 1 {
			t.Fatalf("global: got %d args, want 1", len(args))
		}
		if args[0] != 10 {
			t.Errorf("global limit = %v", args[0])
		}
		if strings.Contains(query, "ended_at >=") {
			t.Error("global query should not have time filter")
		}
	})

	t.Run("weekly scope", func(t *testing.T) {
		query, args := leaderboardQuery("weekly", 5)
		if len(args) != 2 {
			t.Fatalf("weekly: got %d args, want 2", len(args))
		}
		if args[1] != 5 {
			t.Errorf("weekly limit = %v", args[1])
		}
		if !strings.Contains(query, "ended_at >=") {
			t.Error("weekly query should have time filter")
		}
	})
}

func TestGetLeaderboard(t *testing.T) {
	tests := []struct {
		name        string
		scope       string
		limit       int
		queryErr    error
		rows        *pgxmock.Rows
		wantEntries int
		wantErr     bool
	}{
		{
			name:        "success",
			scope:       "all",
			limit:       50,
			rows:        pgxmock.NewRows([]string{"final_score", "lobby_code", "ended_at"}).AddRow(1000, "CODE1", int64(100)).AddRow(800, "CODE2", int64(200)),
			wantEntries: 2,
		},
		{
			name:        "empty returns slice",
			scope:       "all",
			limit:       0, // internally replaced with 50
			rows:        pgxmock.NewRows([]string{"final_score", "lobby_code", "ended_at"}),
			wantEntries: 0,
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
				mock.ExpectQuery("SELECT final_score, lobby_code, ended_at FROM game_sessions").WillReturnError(tt.queryErr)
			} else {
				mock.ExpectQuery("SELECT final_score, lobby_code, ended_at FROM game_sessions").WithArgs(effectiveLimit).WillReturnRows(tt.rows)
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
		})
	}
}

func TestGetUserBestScore(t *testing.T) {
	tests := []struct {
		name       string
		queryErr   error
		wantScore  int
		wantGames  int
		wantErr    bool
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
