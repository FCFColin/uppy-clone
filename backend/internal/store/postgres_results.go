package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// CreateGameSession inserts a new game session record.
// No retry: non-idempotent.
func (s *PostgresStore) CreateGameSession(ctx context.Context, gs *domain.GameSession) error {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.CreateGameSession",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "INSERT"),
		),
	)
	defer span.End()

	return s.withRetryWrite(ctx, func(ctx context.Context) error {
		_, execErr := s.pool.Exec(ctx,
			`INSERT INTO game_sessions (id, lobby_code, created_by, status, started_at, ended_at, final_score) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			gs.ID, gs.LobbyCode, gs.CreatedBy, gs.Status, gs.StartedAt, gs.EndedAt, gs.FinalScore)
		if execErr != nil {
			return fmt.Errorf("create game session: %w", execErr)
		}
		return nil
	})
}

// EndGameAndRecordResults ends a game session and records all player results in one transaction.
func (s *PostgresStore) EndGameAndRecordResults(ctx context.Context, sessionID string, endedAt int64, finalScore int, results []domain.GameResult) error {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.EndGameAndRecordResults",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.session_id", sessionID),
			attribute.Int("db.results_count", len(results)),
		),
	)
	defer span.End()

	_, err := s.cb.Execute(func() (any, error) {
		tx, txErr := s.pool.Begin(ctx)
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

// GetGameResultsByUserID returns the most recent game results for a user.
func (s *PostgresStore) GetGameResultsByUserID(ctx context.Context, userID string) ([]domain.GameResult, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.GetGameResultsByUserID",
		trace.WithAttributes(attribute.String("db.system", "postgresql")),
	)
	defer span.End()

	var results []domain.GameResult
	err := s.withRetryRead(ctx, func(ctx context.Context) error {
		rows, err := s.pool.Query(ctx,
			`SELECT id, session_id, user_id, score_contribution, taps_count, created_at FROM game_results WHERE user_id = $1 ORDER BY created_at DESC LIMIT 100`, userID)
		if err != nil {
			return fmt.Errorf("query game results: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var r domain.GameResult
			if scanErr := rows.Scan(&r.ID, &r.SessionID, &r.UserID, &r.ScoreContribution, &r.TapsCount, &r.CreatedAt); scanErr != nil {
				return fmt.Errorf("scan game result: %w", scanErr)
			}
			results = append(results, r)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}
