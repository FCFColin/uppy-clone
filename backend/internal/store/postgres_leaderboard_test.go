package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
)

type mockLeaderboardRows struct {
	scores   []int
	codes    []string
	endedAts []int64
	pos      int
	closed   bool
	err      error
	scanErr  error
}

func (m *mockLeaderboardRows) Close()                                       { m.closed = true }
func (m *mockLeaderboardRows) Err() error                                   { return m.err }
func (m *mockLeaderboardRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (m *mockLeaderboardRows) Conn() *pgx.Conn                              { return nil }
func (m *mockLeaderboardRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (m *mockLeaderboardRows) Next() bool {
	if m.err != nil || m.pos >= len(m.scores) {
		return false
	}
	m.pos++
	return m.pos <= len(m.scores)
}
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
func (m *mockLeaderboardRows) RawValues() [][]byte    { return nil }
func (m *mockLeaderboardRows) Values() ([]any, error) { return nil, nil }

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
			scores:  []int{100},
			codes:   []string{"C"},
			scanErr: errors.New("field type mismatch"),
		}
		_, err := scanLeaderboardRows(rows)
		if err == nil || !strings.Contains(err.Error(), "scan leaderboard row") {
			t.Errorf("expected scan error, got %v", err)
		}
	})

	t.Run("rows.Err propagates", func(t *testing.T) {
		rows := &mockLeaderboardRows{
			scores: []int{100},
			codes:  []string{"D"},
			err:    errors.New("connection lost"),
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

func TestGetLeaderboard_Success(t *testing.T) {
	repo, mock := newMockResultRepository(t)
	ctx := context.Background()

	rows := pgxmock.NewRows([]string{"final_score", "lobby_code", "ended_at"}).
		AddRow(1000, "CODE1", int64(100)).
		AddRow(800, "CODE2", int64(200))
	mock.ExpectQuery("SELECT final_score, lobby_code, ended_at FROM game_sessions").
		WithArgs(50).
		WillReturnRows(rows)

	entries, err := repo.GetLeaderboard(ctx, "all", 50)
	if err != nil {
		t.Fatalf("GetLeaderboard: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
}

func TestGetLeaderboard_EmptyReturnsSlice(t *testing.T) {
	repo, mock := newMockResultRepository(t)
	ctx := context.Background()

	rows := pgxmock.NewRows([]string{"final_score", "lobby_code", "ended_at"})
	mock.ExpectQuery("SELECT final_score, lobby_code, ended_at FROM game_sessions").
		WithArgs(50).
		WillReturnRows(rows)

	entries, err := repo.GetLeaderboard(ctx, "all", 0)
	if err != nil {
		t.Fatalf("GetLeaderboard: %v", err)
	}
	if entries == nil {
		t.Fatal("expected non-nil slice")
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestGetLeaderboard_QueryError(t *testing.T) {
	repo, mock := newMockResultRepository(t)
	ctx := context.Background()

	mock.ExpectQuery("SELECT final_score, lobby_code, ended_at FROM game_sessions").
		WillReturnError(errors.New("query failed"))

	_, err := repo.GetLeaderboard(ctx, "weekly", 10)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetUserBestScore_Success(t *testing.T) {
	repo, mock := newMockResultRepository(t)
	ctx := context.Background()

	rows := pgxmock.NewRows([]string{"max", "count"}).
		AddRow(500, 3)
	mock.ExpectQuery("SELECT COALESCE").WithArgs("user-1").WillReturnRows(rows)

	score, games, err := repo.GetUserBestScore(ctx, "user-1")
	if err != nil {
		t.Fatalf("GetUserBestScore: %v", err)
	}
	if score != 500 || games != 3 {
		t.Fatalf("GetUserBestScore = (%d,%d), want (500,3)", score, games)
	}
}

func TestGetUserBestScore_Error(t *testing.T) {
	repo, mock := newMockResultRepository(t)
	ctx := context.Background()

	mock.ExpectQuery("SELECT COALESCE").WillReturnError(errors.New("query failed"))

	_, _, err := repo.GetUserBestScore(ctx, "user-1")
	if err == nil {
		t.Fatal("expected error")
	}
}