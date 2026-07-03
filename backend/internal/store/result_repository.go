package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ResultRepository handles game session and result persistence.
type ResultRepository struct {
	baseRepository
}

// NewResultRepository creates a ResultRepository.
func NewResultRepository(pool *pgxpool.Pool) *ResultRepository {
	return &ResultRepository{baseRepository: newBaseRepository(pool)}
}

func (r *ResultRepository) CreateGameSession(ctx context.Context, gs *domain.GameSession) error {
	ctx, span := telemetry.Tracer().Start(ctx, "result_repo.CreateGameSession",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "INSERT"),
		),
	)
	defer span.End()

	return r.withRetryWrite(ctx, func(ctx context.Context) error {
		_, execErr := r.pool.Exec(ctx,
			`INSERT INTO game_sessions (id, lobby_code, created_by, status, started_at, ended_at, final_score) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			gs.ID, gs.LobbyCode, gs.CreatedBy, gs.Status, gs.StartedAt, gs.EndedAt, gs.FinalScore)
		if execErr != nil {
			return fmt.Errorf("create game session: %w", execErr)
		}
		return nil
	})
}

func (r *ResultRepository) RecordGameResult(ctx context.Context, sessionID, roomCode string, endedAt int64, finalScore int, results []domain.GameResultPlayer) error {
	ctx, span := telemetry.Tracer().Start(ctx, "result_repo.RecordGameResult",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.session_id", sessionID),
		),
	)
	defer span.End()

	return r.withRetryWrite(ctx, func(ctx context.Context) error {
		tx, txErr := r.pool.Begin(ctx)
		if txErr != nil {
			return fmt.Errorf("begin tx: %w", txErr)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		if _, err := tx.Exec(ctx,
			`INSERT INTO game_sessions (id, lobby_code, status, ended_at, final_score)
			 VALUES ($1, $2, 'ended', $3, $4)
			 ON CONFLICT (id) DO UPDATE SET status = 'ended', ended_at = EXCLUDED.ended_at, final_score = EXCLUDED.final_score`,
			sessionID, roomCode, endedAt, finalScore); err != nil {
			return fmt.Errorf("upsert game session: %w", err)
		}

		for _, pr := range results {
			resultID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(sessionID+pr.UserID)).String()
			if _, err := tx.Exec(ctx,
				`INSERT INTO game_results (id, session_id, user_id, score_contribution, taps_count, created_at)
				 VALUES ($1, $2, $3, $4, $5, $6)
				 ON CONFLICT (id) DO NOTHING`,
				resultID, sessionID, pr.UserID, pr.ScoreContribution, pr.TapsCount, endedAt); err != nil {
				return fmt.Errorf("insert game result: %w", err)
			}
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit record game result: %w", err)
		}
		return nil
	})
}

func (r *ResultRepository) EndGameAndRecordResults(ctx context.Context, sessionID string, endedAt int64, finalScore int, results []domain.GameResult) error {
	ctx, span := telemetry.Tracer().Start(ctx, "result_repo.EndGameAndRecordResults",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.session_id", sessionID),
			attribute.Int("db.results_count", len(results)),
		),
	)
	defer span.End()

	_, err := r.cb.Execute(func() (any, error) {
		tx, txErr := r.pool.Begin(ctx)
		if txErr != nil {
			return nil, fmt.Errorf("begin tx: %w", txErr)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		if _, execErr := tx.Exec(ctx,
			`UPDATE game_sessions SET status = 'ended', ended_at = $1, final_score = $2 WHERE id = $3`,
			endedAt, finalScore, sessionID); execErr != nil {
			return nil, fmt.Errorf("end game session: %w", execErr)
		}

		if len(results) > 0 {
			var placeholders []string
			var values []interface{}
			for i, r := range results {
				base := i * 6
				placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d)", base+1, base+2, base+3, base+4, base+5, base+6))
				values = append(values, r.ID, r.SessionID, r.UserID, r.ScoreContribution, r.TapsCount, r.CreatedAt)
			}
			query := fmt.Sprintf("INSERT INTO game_results (id, session_id, user_id, score_contribution, taps_count, created_at) VALUES %s ON CONFLICT (id) DO NOTHING", strings.Join(placeholders, ","))
			if _, execErr := tx.Exec(ctx, query, values...); execErr != nil {
				return nil, fmt.Errorf("insert game results: %w", execErr)
			}
		}

		if commitErr := tx.Commit(ctx); commitErr != nil {
			return nil, fmt.Errorf("commit end game and results: %w", commitErr)
		}
		return nil, nil
	})
	return err
}

func (r *ResultRepository) InsertSeedGameResult(ctx context.Context, result *domain.GameResult) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO game_results (id, session_id, user_id, score_contribution, taps_count, created_at) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (id) DO NOTHING`,
		result.ID, result.SessionID, result.UserID, result.ScoreContribution, result.TapsCount, result.CreatedAt)
	return err
}

func (r *ResultRepository) GetLeaderboard(ctx context.Context, scope string, limit int) ([]domain.LeaderboardEntry, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "result_repo.GetLeaderboard",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("leaderboard.scope", scope),
		),
	)
	defer span.End()

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var entries []domain.LeaderboardEntry
	err := r.withRetryRead(ctx, func(ctx context.Context) error {
		query, args := leaderboardQuery(scope, limit)
		rows, err := r.pool.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("query leaderboard: %w", err)
		}
		entries, err = scanLeaderboardRows(rows)
		return err
	})
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []domain.LeaderboardEntry{}
	}
	return entries, nil
}

func (r *ResultRepository) GetUserBestScore(ctx context.Context, userID string) (int, int, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "result_repo.GetUserBestScore",
		trace.WithAttributes(attribute.String("db.system", "postgresql")),
	)
	defer span.End()

	var bestScore int
	var gamesPlayed int
	err := r.withRetryRead(ctx, func(ctx context.Context) error {
		return r.pool.QueryRow(ctx,
			`SELECT COALESCE(MAX(score_contribution), 0), COUNT(*)
			 FROM game_results WHERE user_id = $1`,
			userID,
		).Scan(&bestScore, &gamesPlayed)
	})
	return bestScore, gamesPlayed, err
}
