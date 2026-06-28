package store

import (
	"context"
	"errors"
	"fmt"
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
		if len(entries) != 2 || entries[0].Rank != 1 || entries[1].Rank != 2 {
			t.Fatalf("entries = %+v", entries)
		}
		if entries[0].Score != 900 || entries[0].LobbyCode != "A" {
			t.Errorf("first entry = %+v", entries[0])
		}
		if !rows.closed {
			t.Error("expected rows to be closed")
		}
	})

	t.Run("scan error", func(t *testing.T) {
		rows := &mockLeaderboardRows{
			scores:   []int{1},
			codes:    []string{"X"},
			endedAts: []int64{1},
			scanErr:  errors.New("scan failed"),
		}
		_, err := scanLeaderboardRows(rows)
		if err == nil {
			t.Fatal("expected scan error")
		}
	})
}

func TestGetLeaderboard_Success(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	rows := pgxmock.NewRows([]string{"final_score", "lobby_code", "ended_at"}).
		AddRow(500, "LOBBY1", int64(1000))
	mock.ExpectQuery("SELECT final_score, lobby_code, ended_at").
		WithArgs(50).
		WillReturnRows(rows)

	entries, err := s.GetLeaderboard(ctx, "all", 50)
	if err != nil {
		t.Fatalf("GetLeaderboard: %v", err)
	}
	if len(entries) != 1 || entries[0].Rank != 1 || entries[0].Score != 500 {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestGetLeaderboard_EmptyReturnsSlice(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	mock.ExpectQuery("SELECT final_score, lobby_code, ended_at").
		WithArgs(50).
		WillReturnRows(pgxmock.NewRows([]string{"final_score", "lobby_code", "ended_at"}))

	entries, err := s.GetLeaderboard(ctx, "all", 0)
	if err != nil {
		t.Fatalf("GetLeaderboard: %v", err)
	}
	if entries == nil || len(entries) != 0 {
		t.Fatalf("entries = %v, want empty slice", entries)
	}
}

func TestGetLeaderboard_QueryError(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	mock.ExpectQuery("SELECT final_score, lobby_code, ended_at").
		WillReturnError(errors.New("query failed"))

	_, err := s.GetLeaderboard(ctx, "weekly", 10)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetUserBestScore_Success(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(score_contribution\\), 0\\), COUNT\\(\\*\\)").
		WithArgs("user-1").
		WillReturnRows(pgxmock.NewRows([]string{"max", "count"}).AddRow(42, 7))

	score, games, err := s.GetUserBestScore(ctx, "user-1")
	if err != nil {
		t.Fatalf("GetUserBestScore: %v", err)
	}
	if score != 42 || games != 7 {
		t.Errorf("score=%d games=%d", score, games)
	}
}

func TestGetUserBestScore_Error(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(score_contribution\\), 0\\), COUNT\\(\\*\\)").
		WillReturnError(errors.New("query failed"))

	_, _, err := s.GetUserBestScore(ctx, "user-1")
	if err == nil {
		t.Fatal("expected error")
	}
}
