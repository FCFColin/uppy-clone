package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/domain"
)

func TestInsertSeedGameResult_Success(t *testing.T) {
	repo, mock := newMockRepo(t, NewResultRepository)
	ctx := context.Background()
	result := &domain.GameResult{
		ID: "r1", SessionID: "s1", UserID: "u1",
		ScoreContribution: 100, TapsCount: 5, CreatedAt: 1000,
	}
	mock.ExpectExec("INSERT INTO game_results").
		WithArgs(result.ID, result.SessionID, result.UserID, result.ScoreContribution, result.TapsCount, result.CreatedAt).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	if err := repo.InsertSeedGameResult(ctx, result); err != nil {
		t.Fatalf("InsertSeedGameResult: %v", err)
	}
}

func TestInsertSeedGameResult_Error(t *testing.T) {
	repo, mock := newMockRepo(t, NewResultRepository)
	ctx := context.Background()

	mock.ExpectExec("INSERT INTO game_results").
		WillReturnError(errors.New("insert failed"))

	err := repo.InsertSeedGameResult(ctx, &domain.GameResult{ID: "r1", SessionID: "s1", UserID: "u1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetGameResultsByUserID_Success(t *testing.T) {
	repo, mock := newMockRepo(t, NewUserRepository)
	ctx := context.Background()

	rows := pgxmock.NewRows([]string{"id", "session_id", "user_id", "score_contribution", "taps_count", "created_at"}).
		AddRow("r1", "s1", "u1", 100, 10, int64(1000)).
		AddRow("r2", "s2", "u1", 50, 5, int64(900))
	mock.ExpectQuery("SELECT id, session_id, user_id, score_contribution, taps_count, created_at FROM game_results").
		WithArgs("u1").
		WillReturnRows(rows)

	results, err := repo.GetGameResultsByUserID(ctx, "u1")
	if err != nil {
		t.Fatalf("GetGameResultsByUserID: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestGetGameResultsByUserID_Empty(t *testing.T) {
	repo, mock := newMockRepo(t, NewUserRepository)
	ctx := context.Background()

	rows := pgxmock.NewRows([]string{"id", "session_id", "user_id", "score_contribution", "taps_count", "created_at"})
	mock.ExpectQuery("SELECT id, session_id, user_id, score_contribution, taps_count, created_at FROM game_results").
		WithArgs("u-empty").
		WillReturnRows(rows)

	results, err := repo.GetGameResultsByUserID(ctx, "u-empty")
	if err != nil {
		t.Fatalf("GetGameResultsByUserID: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestCreateGameSession_Success(t *testing.T) {
	repo, mock := newMockRepo(t, NewResultRepository)
	ctx := context.Background()

	startedAt := int64(100)
	createdBy := "u1"
	gs := &domain.GameSession{
		ID: "s1", LobbyCode: "CODE1", CreatedBy: &createdBy,
		Status: "active", StartedAt: &startedAt,
	}

	mock.ExpectExec("INSERT INTO game_sessions").
		WithArgs(gs.ID, gs.LobbyCode, gs.CreatedBy, gs.Status, gs.StartedAt, gs.EndedAt, gs.FinalScore).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))

	if err := repo.CreateGameSession(ctx, gs); err != nil {
		t.Fatalf("CreateGameSession: %v", err)
	}
}

func TestRecordGameResult_Success(t *testing.T) {
	repo, mock := newMockRepo(t, NewResultRepository)
	ctx := context.Background()

	results := []domain.GameResultPlayer{
		{UserID: "u1", ScoreContribution: 50, TapsCount: 5},
		{UserID: "u2", ScoreContribution: 30, TapsCount: 3},
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO game_sessions").
		WithArgs("sess-1", "CODE1", int64(200), 100).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectExec("INSERT INTO game_results").
		WithArgs(pgxmock.AnyArg(), "sess-1", "u1", 50, 5, int64(200), pgxmock.AnyArg(), "sess-1", "u2", 30, 3, int64(200)).
		WillReturnResult(pgconn.NewCommandTag("INSERT 2"))
	mock.ExpectCommit()

	if err := repo.RecordGameResult(ctx, "sess-1", "CODE1", 200, 100, results); err != nil {
		t.Fatalf("RecordGameResult: %v", err)
	}
}

func TestRecordGameResult_EmptyResults(t *testing.T) {
	repo, mock := newMockRepo(t, NewResultRepository)
	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO game_sessions").
		WithArgs("sess-1", "CODE1", int64(200), 100).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectCommit()

	if err := repo.RecordGameResult(ctx, "sess-1", "CODE1", 200, 100, nil); err != nil {
		t.Fatalf("RecordGameResult: %v", err)
	}
}
