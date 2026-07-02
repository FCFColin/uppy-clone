package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/domain"
)

func strPtr(s string) *string { return &s }

func int64Ptr(n int64) *int64 { return &n }

func TestInsertSeedGameResult_Success(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()
	result := &domain.GameResult{
		ID: "r1", SessionID: "s1", UserID: "u1",
		ScoreContribution: 100, TapsCount: 5, CreatedAt: 1000,
	}
	mock.ExpectExec("INSERT INTO game_results").
		WithArgs(result.ID, result.SessionID, result.UserID, result.ScoreContribution, result.TapsCount, result.CreatedAt).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	if err := s.InsertSeedGameResult(ctx, result); err != nil {
		t.Fatalf("InsertSeedGameResult: %v", err)
	}
}

func TestCreateGameSession_Success(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()
	gs := &domain.GameSession{
		ID: "sess-1", LobbyCode: "ABCD1", CreatedBy: strPtr("user-1"),
		Status: "active", StartedAt: int64Ptr(100), EndedAt: int64Ptr(0), FinalScore: 0,
	}

	mock.ExpectExec("INSERT INTO game_sessions").
		WithArgs(gs.ID, gs.LobbyCode, gs.CreatedBy, gs.Status, gs.StartedAt, gs.EndedAt, gs.FinalScore).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))

	if err := s.CreateGameSession(ctx, gs); err != nil {
		t.Fatalf("CreateGameSession: %v", err)
	}
}

func TestCreateGameSession_Error(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	mock.ExpectExec("INSERT INTO game_sessions").
		WillReturnError(errors.New("insert failed"))

	err := s.CreateGameSession(ctx, &domain.GameSession{ID: "s1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEndGameAndRecordResults_NoResults(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE game_sessions SET status").
		WithArgs(int64(200), 100, "sess-1").
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))
	mock.ExpectCommit()

	if err := s.EndGameAndRecordResults(ctx, "sess-1", 200, 100, nil); err != nil {
		t.Fatalf("EndGameAndRecordResults: %v", err)
	}
}

func TestEndGameAndRecordResults_WithResults(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()
	results := []domain.GameResult{
		{ID: "r1", SessionID: "sess-1", UserID: "u1", ScoreContribution: 50, TapsCount: 10, CreatedAt: 200},
	}

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE game_sessions SET status").
		WithArgs(int64(200), 100, "sess-1").
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))
	mock.ExpectExec("INSERT INTO game_results").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectCommit()

	if err := s.EndGameAndRecordResults(ctx, "sess-1", 200, 100, results); err != nil {
		t.Fatalf("EndGameAndRecordResults: %v", err)
	}
}

func TestEndGameAndRecordResults_BeginError(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	mock.ExpectBegin().WillReturnError(errors.New("begin failed"))

	err := s.EndGameAndRecordResults(ctx, "sess-1", 200, 100, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetGameResultsByUserID_Success(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	rows := pgxmock.NewRows([]string{"id", "session_id", "user_id", "score_contribution", "taps_count", "created_at"}).
		AddRow("r1", "sess-1", "user-1", 10, 5, int64(100))
	mock.ExpectQuery("SELECT id, session_id, user_id, score_contribution, taps_count, created_at FROM game_results").
		WithArgs("user-1").
		WillReturnRows(rows)

	results, err := s.GetGameResultsByUserID(ctx, "user-1")
	if err != nil {
		t.Fatalf("GetGameResultsByUserID: %v", err)
	}
	if len(results) != 1 || results[0].ID != "r1" {
		t.Fatalf("results = %+v", results)
	}
}

func TestRecordGameResult_Success(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()
	results := []domain.GameResultPlayer{
		{UserID: "user-1", ScoreContribution: 50, TapsCount: 10},
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO game_sessions").
		WithArgs("sess-1", "ROOM1", int64(200), 100).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectExec("INSERT INTO game_results").
		WithArgs(pgxmock.AnyArg(), "sess-1", "user-1", 50, 10, int64(200)).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectCommit()

	if err := s.RecordGameResult(ctx, "sess-1", "ROOM1", 200, 100, results); err != nil {
		t.Fatalf("RecordGameResult: %v", err)
	}
}

func TestRecordGameResult_BeginError(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	mock.ExpectBegin().WillReturnError(errors.New("begin failed"))

	err := s.RecordGameResult(ctx, "sess-1", "ROOM1", 200, 100, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRecordGameResult_UpsertError(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO game_sessions").
		WillReturnError(errors.New("upsert failed"))
	mock.ExpectRollback()

	err := s.RecordGameResult(ctx, "sess-1", "ROOM1", 200, 100, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRecordGameResult_CommitError(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO game_sessions").
		WithArgs("sess-1", "ROOM1", int64(200), 100).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
	mock.ExpectRollback()

	err := s.RecordGameResult(ctx, "sess-1", "ROOM1", 200, 100, nil)
	if err == nil || !strings.Contains(err.Error(), "commit record game result") {
		t.Fatalf("RecordGameResult = %v, want commit error", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRecordGameResult_InsertResultError(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()
	results := []domain.GameResultPlayer{{UserID: "user-1", ScoreContribution: 10, TapsCount: 1}}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO game_sessions").
		WithArgs("sess-1", "ROOM1", int64(200), 100).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectExec("INSERT INTO game_results").
		WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()

	err := s.RecordGameResult(ctx, "sess-1", "ROOM1", 200, 100, results)
	if err == nil || !strings.Contains(err.Error(), "insert game result") {
		t.Fatalf("RecordGameResult = %v, want insert error", err)
	}
}

func TestGetGameResultsByUserID_QueryError(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	mock.ExpectQuery("SELECT id, session_id, user_id, score_contribution, taps_count, created_at FROM game_results").
		WillReturnError(errors.New("query failed"))

	_, err := s.GetGameResultsByUserID(ctx, "user-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetGameResultsByUserID_ScanError(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	rows := pgxmock.NewRows([]string{"id", "session_id", "user_id", "score_contribution", "taps_count", "created_at"}).
		AddRow("r1", "sess-1", "user-1", "bad", 5, int64(100))
	mock.ExpectQuery("SELECT id, session_id, user_id, score_contribution, taps_count, created_at FROM game_results").
		WithArgs("user-1").
		WillReturnRows(rows)

	_, err := s.GetGameResultsByUserID(ctx, "user-1")
	if err == nil {
		t.Fatal("expected scan error")
	}
}

func TestInsertSeedGameResult_Error(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	mock.ExpectExec("INSERT INTO game_results").
		WillReturnError(errors.New("insert failed"))

	err := s.InsertSeedGameResult(ctx, &domain.GameResult{ID: "r1", SessionID: "s1", UserID: "u1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEndGameAndRecordResults_UpdateError(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE game_sessions SET status").
		WillReturnError(errors.New("update failed"))
	mock.ExpectRollback()

	err := s.EndGameAndRecordResults(ctx, "sess-1", 200, 100, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEndGameAndRecordResults_InsertBatchError(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()
	results := []domain.GameResult{
		{ID: "r1", SessionID: "sess-1", UserID: "u1", ScoreContribution: 10, TapsCount: 1, CreatedAt: 200},
	}

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE game_sessions SET status").
		WithArgs(int64(200), 100, "sess-1").
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))
	mock.ExpectExec("INSERT INTO game_results").
		WillReturnError(errors.New("batch insert failed"))
	mock.ExpectRollback()

	err := s.EndGameAndRecordResults(ctx, "sess-1", 200, 100, results)
	if err == nil || !strings.Contains(err.Error(), "insert game results") {
		t.Fatalf("EndGameAndRecordResults = %v, want batch insert error", err)
	}
}

func TestEndGameAndRecordResults_CommitError(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE game_sessions SET status").
		WithArgs(int64(200), 100, "sess-1").
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))
	mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
	mock.ExpectRollback()

	err := s.EndGameAndRecordResults(ctx, "sess-1", 200, 100, nil)
	if err == nil || !strings.Contains(err.Error(), "commit end game and results") {
		t.Fatalf("EndGameAndRecordResults = %v, want commit error", err)
	}
}
