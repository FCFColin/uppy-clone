package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/util"
	"go.opentelemetry.io/otel/attribute"
)

// gameResultNamespace is a custom UUID namespace for generating deterministic game result IDs.
// Using a fixed custom namespace (not NameSpaceDNS) ensures no collision with RFC 4122 reserved namespaces.
var gameResultNamespace = uuid.MustParse("a6e0e8e0-3b9c-4a5e-8f1d-2c3b4a5e6f7d")

// ResultRepository handles game session and result persistence.
type ResultRepository struct {
	baseRepository
}

// NewResultRepository creates a ResultRepository.
func NewResultRepository(pool pgPool, deps ...Deps) *ResultRepository {
	d := depsOrZero(deps...)
	return &ResultRepository{baseRepository: newBaseRepository(pool, d)}
}

// CreateGameSession inserts a new game session record into the database.
func (r *ResultRepository) CreateGameSession(ctx context.Context, gs *domain.GameSession) error {
	ctx, span := withSpan(ctx, r.deps.Tracer, "result_repo.CreateGameSession",
		attribute.String("db.operation", "INSERT"),
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

// RecordGameResult records the final results of a game session.
func (r *ResultRepository) RecordGameResult(ctx context.Context, sessionID, roomCode string, endedAt int64, finalScore int, results []domain.GameResultPlayer) error {
	ctx, span := withSpan(ctx, r.deps.Tracer, "result_repo.RecordGameResult",
		attribute.String("db.session_id", sessionID),
	)
	defer span.End()

	err := r.withRetryWrite(ctx, func(ctx context.Context) error {
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

		if len(results) > 0 {
			var placeholders []string
			var values []interface{}
			for i, pr := range results {
				base := i * 6
				placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d)", base+1, base+2, base+3, base+4, base+5, base+6))
				resultID := uuid.NewSHA1(gameResultNamespace, []byte(sessionID+"\x00"+pr.UserID)).String()
				values = append(values, resultID, sessionID, pr.UserID, pr.ScoreContribution, pr.TapsCount, endedAt)
			}
			query := fmt.Sprintf("INSERT INTO game_results (id, session_id, user_id, score_contribution, taps_count, created_at) VALUES %s ON CONFLICT (id) DO NOTHING", strings.Join(placeholders, ","))
			if _, err := tx.Exec(ctx, query, values...); err != nil {
				return fmt.Errorf("insert game results: %w", err)
			}
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit record game result: %w", err)
		}
		return nil
	})
	if err != nil {
		util.LoggerFromContext(ctx).Error("record game result failed",
			"error", err, "session_id", sessionID)
		return err
	}
	return nil
}

// InsertSeedGameResult inserts a seed game result with retry logic.
// store-025: Use withRetryWrite (consistent with RecordGameResult and
// EndGameAndRecordResults) instead of calling cb.Execute directly, so retry
// backoff/max-retries are unified across all write entry points.
func (r *ResultRepository) InsertSeedGameResult(ctx context.Context, result *domain.GameResult) error {
	return r.withRetryWrite(ctx, func(ctx context.Context) error {
		_, execErr := r.pool.Exec(ctx,
			`INSERT INTO game_results (id, session_id, user_id, score_contribution, taps_count, created_at) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (id) DO NOTHING`,
			result.ID, result.SessionID, result.UserID, result.ScoreContribution, result.TapsCount, result.CreatedAt)
		return execErr
	})
}

// GetLeaderboard returns the top entries for the given scope and limit.
func (r *ResultRepository) GetLeaderboard(ctx context.Context, scope string, limit int) ([]domain.LeaderboardEntry, error) {
	ctx, span := withSpan(ctx, r.deps.Tracer, "result_repo.GetLeaderboard",
		attribute.String("leaderboard.scope", scope),
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

// GetUserBestScore returns the best score and games played for the given user.
func (r *ResultRepository) GetUserBestScore(ctx context.Context, userID string) (int, int, error) {
	ctx, span := withSpan(ctx, r.deps.Tracer, "result_repo.GetUserBestScore")
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

func leaderboardQuery(scope string, limit int) (string, []interface{}) {
	if scope == "weekly" {
		cutoff := time.Now().Add(-7 * 24 * time.Hour).UnixMilli()
		return `SELECT final_score, lobby_code, ended_at
			FROM game_sessions
			WHERE status = 'ended' AND final_score > 0 AND ended_at >= $1
			ORDER BY final_score DESC, ended_at ASC
			LIMIT $2`, []interface{}{cutoff, limit}
	}
	return `SELECT final_score, lobby_code, ended_at
		FROM game_sessions
		WHERE status = 'ended' AND final_score > 0
		ORDER BY final_score DESC, ended_at ASC
		LIMIT $1`, []interface{}{limit}
}

func scanLeaderboardRows(rows pgx.Rows) ([]domain.LeaderboardEntry, error) {
	var entries []domain.LeaderboardEntry
	rank := 1
	for rows.Next() {
		var score int
		var lobbyCode string
		var endedAt int64
		if scanErr := rows.Scan(&score, &lobbyCode, &endedAt); scanErr != nil {
			return nil, fmt.Errorf("scan leaderboard row: %w", scanErr)
		}
		entries = append(entries, domain.LeaderboardEntry{
			Rank:      rank,
			Score:     score,
			LobbyCode: lobbyCode,
			EndedAt:   endedAt,
		})
		rank++
	}
	return entries, rows.Err()
}
