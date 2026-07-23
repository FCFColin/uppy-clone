package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/domain"
)

func TestInsertSeedGameResult(t *testing.T) {
	result := &domain.GameResult{
		ID: "r1", SessionID: "s1", UserID: "u1",
		ScoreContribution: 100, TapsCount: 5, CreatedAt: 1000,
	}
	tests := []struct {
		name     string
		queryErr error
		wantErr  bool
	}{
		{"success", nil, false},
		{"insert error", errors.New("insert failed"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockRepo(t, NewResultRepository)
			ctx := context.Background()

			if tt.queryErr != nil {
				mock.ExpectExec("INSERT INTO game_results").WillReturnError(tt.queryErr)
			} else {
				mock.ExpectExec("INSERT INTO game_results").
					WithArgs(result.ID, result.SessionID, result.UserID, result.ScoreContribution, result.TapsCount, result.CreatedAt).
					WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
			}

			err := repo.InsertSeedGameResult(ctx, result)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("InsertSeedGameResult: %v", err)
			}
		})
	}
}

func TestGetGameResultsByUserID(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		rows        *pgxmock.Rows
		wantResults int
	}{
		{
			name:   "success",
			userID: "u1",
			rows: pgxmock.NewRows([]string{"id", "session_id", "user_id", "score_contribution", "taps_count", "created_at"}).
				AddRow("r1", "s1", "u1", 100, 10, int64(1000)).
				AddRow("r2", "s2", "u1", 50, 5, int64(900)),
			wantResults: 2,
		},
		{
			name:        "empty",
			userID:      "u-empty",
			rows:        pgxmock.NewRows([]string{"id", "session_id", "user_id", "score_contribution", "taps_count", "created_at"}),
			wantResults: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockRepo(t, NewUserRepository)
			ctx := context.Background()

			mock.ExpectQuery("SELECT id, session_id, user_id, score_contribution, taps_count, created_at FROM game_results").
				WithArgs(tt.userID).
				WillReturnRows(tt.rows)

			results, err := repo.GetGameResultsByUserID(ctx, tt.userID)
			if err != nil {
				t.Fatalf("GetGameResultsByUserID: %v", err)
			}
			if len(results) != tt.wantResults {
				t.Fatalf("expected %d results, got %d", tt.wantResults, len(results))
			}
		})
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
		{UserID: "u1", Nickname: "Alice", ScoreContribution: 50, TapsCount: 5},
		{UserID: "u2", Nickname: "Bob", ScoreContribution: 30, TapsCount: 3},
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO game_sessions").
		WithArgs("sess-1", "CODE1", int64(200), 100).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectExec("INSERT INTO game_results").
		WithArgs(pgxmock.AnyArg(), "sess-1", "u1", "Alice", 50, 5, int64(200), pgxmock.AnyArg(), "sess-1", "u2", "Bob", 30, 3, int64(200)).
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

func TestGetGamesTodayCount(t *testing.T) {
	tests := []struct {
		name      string
		rows      *pgxmock.Rows
		queryErr  error
		wantCount int
		wantErr   bool
	}{
		{
			name:      "success with count",
			rows:      pgxmock.NewRows([]string{"count"}).AddRow(7),
			wantCount: 7,
		},
		{
			name:      "success zero count",
			rows:      pgxmock.NewRows([]string{"count"}).AddRow(0),
			wantCount: 0,
		},
		{
			name:     "query error",
			queryErr: errors.New("db unavailable"),
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockRepo(t, NewResultRepository)
			ctx := context.Background()

			expect := mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM game_sessions").
				WithArgs(pgxmock.AnyArg())
			if tt.queryErr != nil {
				expect.WillReturnError(tt.queryErr)
			} else {
				expect.WillReturnRows(tt.rows)
			}

			count, err := repo.GetGamesTodayCount(ctx)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("GetGamesTodayCount: %v", err)
			}
			if !tt.wantErr && count != tt.wantCount {
				t.Errorf("count = %d, want %d", count, tt.wantCount)
			}
		})
	}
}

func TestGetBestScore(t *testing.T) {
	tests := []struct {
		name     string
		rows     *pgxmock.Rows
		queryErr error
		wantBest int
		wantErr  bool
	}{
		{
			name:     "success with max",
			rows:     pgxmock.NewRows([]string{"best"}).AddRow(250),
			wantBest: 250,
		},
		{
			name:     "no records returns zero via coalesce",
			rows:     pgxmock.NewRows([]string{"best"}).AddRow(0),
			wantBest: 0,
		},
		{
			name:     "query error",
			queryErr: errors.New("db unavailable"),
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockRepo(t, NewResultRepository)
			ctx := context.Background()

			expect := mock.ExpectQuery("SELECT COALESCE\\(MAX\\(score_contribution\\), 0\\) FROM game_results")
			if tt.queryErr != nil {
				expect.WillReturnError(tt.queryErr)
			} else {
				expect.WillReturnRows(tt.rows)
			}

			best, err := repo.GetBestScore(ctx)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("GetBestScore: %v", err)
			}
			if !tt.wantErr && best != tt.wantBest {
				t.Errorf("best = %d, want %d", best, tt.wantBest)
			}
		})
	}
}
