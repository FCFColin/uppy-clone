package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/util"
	"go.opentelemetry.io/otel/attribute"
)

type LobbyRepository struct {
	baseRepository
}

func NewLobbyRepository(pool pgPool, deps ...Deps) *LobbyRepository {
	d := depsOrZero(deps...)
	return &LobbyRepository{baseRepository: newBaseRepository(pool, d)}
}

func (r *LobbyRepository) SaveLobbyState(ctx context.Context, ls *domain.LobbyState) error {
	ctx, span := withSpan(ctx, r.deps.Tracer, "lobby_repo.SaveLobbyState",
		attribute.String("lobby.code", ls.Code),
	)
	defer span.End()

	err := r.withRetry(ctx, func(ctx context.Context) error {
		_, execErr := r.pool.Exec(ctx,
			`INSERT INTO lobby_states (id, code, state, updated_at, created_at) VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (code) DO UPDATE SET state = EXCLUDED.state, updated_at = EXCLUDED.updated_at`,
			ls.ID, ls.Code, ls.State, ls.UpdatedAt, ls.CreatedAt)
		if execErr != nil {
			return fmt.Errorf("save lobby state: %w", execErr)
		}
		return nil
	})
	if err != nil {
		util.LoggerFromContext(ctx).Error("save lobby state failed",
			"error", err, "lobby_code", ls.Code)
		return err
	}
	return nil
}

func (r *LobbyRepository) LoadLobbyState(ctx context.Context, code string) (*domain.LobbyState, error) {
	ctx, span := withSpan(ctx, r.deps.Tracer, "lobby_repo.LoadLobbyState",
		attribute.String("lobby.code", code),
	)
	defer span.End()

	var ls *domain.LobbyState
	err := r.withRetry(ctx, func(ctx context.Context) error {
		row := r.pool.QueryRow(ctx,
			`SELECT id, code, state, updated_at, created_at FROM lobby_states WHERE code = $1`, code)

		var lobby domain.LobbyState
		if scanErr := row.Scan(&lobby.ID, &lobby.Code, &lobby.State, &lobby.UpdatedAt, &lobby.CreatedAt); scanErr != nil {
			if errors.Is(scanErr, pgx.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("load lobby state: %w", scanErr)
		}
		ls = &lobby
		return nil
	})
	if err != nil {
		util.LoggerFromContext(ctx).Error("load lobby state failed",
			"error", err, "lobby_code", code)
		return nil, err
	}
	return ls, nil
}

func (r *LobbyRepository) DeleteLobbyState(ctx context.Context, code string) error {
	ctx, span := withSpan(ctx, r.deps.Tracer, "lobby_repo.DeleteLobbyState",
		attribute.String("db.operation", "DELETE"),
	)
	defer span.End()

	return r.withRetry(ctx, func(ctx context.Context) error {
		_, execErr := r.pool.Exec(ctx, `DELETE FROM lobby_states WHERE code = $1`, code)
		if execErr != nil {
			return fmt.Errorf("delete lobby state: %w", execErr)
		}
		return nil
	})
}

func (r *LobbyRepository) LoadAllActiveLobbies(ctx context.Context, limit int, cursor string) (*domain.LobbyListResult, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	spanAttrs := []attribute.KeyValue{
		attribute.Int("db.limit", limit),
	}
	if cursor != "" {
		spanAttrs = append(spanAttrs, attribute.String("db.cursor", cursor))
	}

	ctx, span := withSpan(ctx, r.deps.Tracer, "lobby_repo.LoadAllActiveLobbies", spanAttrs...)
	defer span.End()

	cursorUpdatedAt, cursorCode, err := parseLobbyCursor(cursor)
	if err != nil {
		return nil, fmt.Errorf("parse lobby cursor: %w", err)
	}

	total, err := r.countAllLobbies(ctx)
	if err != nil {
		return nil, err
	}

	lobbies, err := r.fetchLobbiesPage(ctx, limit+1, cursorUpdatedAt, cursorCode)
	if err != nil {
		return nil, err
	}

	return buildLobbyListResult(lobbies, total, limit), nil
}

// countAllLobbies returns an estimated row count from pg_class.reltuples to avoid
// a full-table COUNT(*) scan (store-014). The estimate is refreshed by ANALYZE;
// for tables with < ~1000 rows the estimate is typically exact.
func (r *LobbyRepository) countAllLobbies(ctx context.Context) (int, error) {
	var total int
	err := r.withRetry(ctx, func(ctx context.Context) error {
		if countErr := r.pool.QueryRow(ctx,
			`SELECT COALESCE(reltuples, 0)::int FROM pg_class WHERE relname = 'lobby_states'`).Scan(&total); countErr != nil {
			return fmt.Errorf("estimate lobby count: %w", countErr)
		}
		return nil
	})
	return total, err
}

func (r *LobbyRepository) fetchLobbiesPage(ctx context.Context, fetchLimit int, cursorUpdatedAt int64, cursorCode string) ([]domain.LobbyState, error) {
	var lobbies []domain.LobbyState
	err := r.withRetry(ctx, func(ctx context.Context) error {
		rows, queryErr := r.queryLobbies(ctx, fetchLimit, cursorUpdatedAt, cursorCode)
		if queryErr != nil {
			return fmt.Errorf("load all lobbies: %w", queryErr)
		}
		defer rows.Close()

		result, err := scanLobbyRows(rows)
		if err != nil {
			return err
		}
		lobbies = result
		return nil
	})
	return lobbies, err
}

func (r *LobbyRepository) queryLobbies(ctx context.Context, fetchLimit int, cursorUpdatedAt int64, cursorCode string) (pgx.Rows, error) {
	if cursorUpdatedAt > 0 {
		return r.pool.Query(ctx,
			`SELECT id, code, state, updated_at, created_at FROM lobby_states
			 WHERE (updated_at, code) < ($1, $2)
			 ORDER BY updated_at DESC, code DESC LIMIT $3`,
			cursorUpdatedAt, cursorCode, fetchLimit)
	}
	return r.pool.Query(ctx,
		`SELECT id, code, state, updated_at, created_at FROM lobby_states
		 ORDER BY updated_at DESC, code DESC LIMIT $1`,
		fetchLimit)
}

// parseLobbyCursor parses a pagination cursor of the form "<unix_millis>|<code>".
func parseLobbyCursor(cursor string) (cursorUpdatedAt int64, cursorCode string, err error) {
	if cursor == "" {
		return 0, "", nil
	}
	parts := strings.SplitN(cursor, "|", 2)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("malformed cursor: missing separator")
	}
	if _, scanErr := fmt.Sscanf(parts[0], "%d", &cursorUpdatedAt); scanErr != nil {
		return 0, "", fmt.Errorf("malformed cursor timestamp: %w", scanErr)
	}
	if cursorUpdatedAt <= 0 || parts[1] == "" {
		return 0, "", fmt.Errorf("malformed cursor: invalid timestamp or empty code")
	}
	return cursorUpdatedAt, parts[1], nil
}

func scanLobbyRows(rows pgx.Rows) ([]domain.LobbyState, error) {
	var result []domain.LobbyState
	for rows.Next() {
		var ls domain.LobbyState
		if scanErr := rows.Scan(&ls.ID, &ls.Code, &ls.State, &ls.UpdatedAt, &ls.CreatedAt); scanErr != nil {
			return nil, fmt.Errorf("scan lobby: %w", scanErr)
		}
		result = append(result, ls)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("iterate lobbies: %w", rowsErr)
	}
	return result, nil
}

func buildLobbyListResult(lobbies []domain.LobbyState, total, limit int) *domain.LobbyListResult {
	hasMore := len(lobbies) > limit
	if hasMore {
		lobbies = lobbies[:limit]
	}
	var nextCursor string
	if hasMore && len(lobbies) > 0 {
		last := lobbies[len(lobbies)-1]
		nextCursor = fmt.Sprintf("%d|%s", last.UpdatedAt, last.Code)
	}
	return &domain.LobbyListResult{
		Lobbies:    lobbies,
		Total:      total,
		HasMore:    hasMore,
		NextCursor: nextCursor,
	}
}

// gameResultNamespace is a custom UUID namespace for generating deterministic game result IDs.
// Using a fixed custom namespace (not NameSpaceDNS) ensures no collision with RFC 4122 reserved namespaces.
var gameResultNamespace = uuid.MustParse("a6e0e8e0-3b9c-4a5e-8f1d-2c3b4a5e6f7d")

type ResultRepository struct {
	baseRepository
}

func NewResultRepository(pool pgPool, deps ...Deps) *ResultRepository {
	d := depsOrZero(deps...)
	return &ResultRepository{baseRepository: newBaseRepository(pool, d)}
}

func (r *ResultRepository) CreateGameSession(ctx context.Context, gs *domain.GameSession) error {
	ctx, span := withSpan(ctx, r.deps.Tracer, "result_repo.CreateGameSession",
		attribute.String("db.operation", "INSERT"),
	)
	defer span.End()

	return r.withRetry(ctx, func(ctx context.Context) error {
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
	ctx, span := withSpan(ctx, r.deps.Tracer, "result_repo.RecordGameResult",
		attribute.String("db.session_id", sessionID),
	)
	defer span.End()

	err := r.withRetry(ctx, func(ctx context.Context) error {
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
				base := i * 7
				placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d)", base+1, base+2, base+3, base+4, base+5, base+6, base+7))
				resultID := uuid.NewSHA1(gameResultNamespace, []byte(sessionID+"\x00"+pr.UserID)).String()
				values = append(values, resultID, sessionID, pr.UserID, pr.Nickname, pr.ScoreContribution, pr.TapsCount, endedAt)
			}
			query := fmt.Sprintf("INSERT INTO game_results (id, session_id, user_id, nickname, score_contribution, taps_count, created_at) VALUES %s ON CONFLICT (id) DO NOTHING", strings.Join(placeholders, ","))
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

// InsertSeedGameResult uses withRetry (store-025) instead of calling cb.Execute directly,
// so retry backoff/max-retries are unified across all write entry points.
func (r *ResultRepository) InsertSeedGameResult(ctx context.Context, result *domain.GameResult) error {
	return r.withRetry(ctx, func(ctx context.Context) error {
		_, execErr := r.pool.Exec(ctx,
			`INSERT INTO game_results (id, session_id, user_id, score_contribution, taps_count, created_at) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (id) DO NOTHING`,
			result.ID, result.SessionID, result.UserID, result.ScoreContribution, result.TapsCount, result.CreatedAt)
		return execErr
	})
}

func (r *ResultRepository) GetLeaderboard(ctx context.Context, scope string, limit int) ([]domain.LeaderboardEntry, error) {
	ctx, span := withSpan(ctx, r.deps.Tracer, "result_repo.GetLeaderboard",
		attribute.String("leaderboard.scope", scope),
	)
	defer span.End()

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var entries []domain.LeaderboardEntry
	err := r.withRetry(ctx, func(ctx context.Context) error {
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
	ctx, span := withSpan(ctx, r.deps.Tracer, "result_repo.GetUserBestScore")
	defer span.End()

	var bestScore int
	var gamesPlayed int
	err := r.withRetry(ctx, func(ctx context.Context) error {
		return r.pool.QueryRow(ctx,
			`SELECT COALESCE(MAX(score_contribution), 0), COUNT(*)
			 FROM game_results WHERE user_id = $1`,
			userID,
		).Scan(&bestScore, &gamesPlayed)
	})
	return bestScore, gamesPlayed, err
}

func (r *ResultRepository) GetGamesTodayCount(ctx context.Context) (int, error) {
	ctx, span := withSpan(ctx, r.deps.Tracer, "result_repo.GetGamesTodayCount")
	defer span.End()

	now := time.Now().UTC()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).UnixMilli()

	var count int
	err := r.withRetry(ctx, func(ctx context.Context) error {
		return r.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM game_sessions WHERE started_at >= $1`,
			startOfDay,
		).Scan(&count)
	})
	return count, err
}

func (r *ResultRepository) GetBestScore(ctx context.Context) (int, error) {
	ctx, span := withSpan(ctx, r.deps.Tracer, "result_repo.GetBestScore")
	defer span.End()

	var best int
	err := r.withRetry(ctx, func(ctx context.Context) error {
		return r.pool.QueryRow(ctx,
			`SELECT COALESCE(MAX(score_contribution), 0) FROM game_results`,
		).Scan(&best)
	})
	return best, err
}

func leaderboardQuery(scope string, limit int) (string, []interface{}) {
	if scope == "weekly" {
		cutoff := time.Now().Add(-7 * 24 * time.Hour).UnixMilli()
		return `SELECT MAX(gr.score_contribution) AS best_score,
		       COALESCE(NULLIF(gr.nickname, ''), gr.user_id::text) AS display_name,
		       MAX(gr.created_at) AS best_at
		FROM game_results gr
		JOIN game_sessions gs ON gs.id = gr.session_id
		WHERE gs.status = 'ended' AND gr.score_contribution > 0 AND gr.created_at >= $1
		GROUP BY gr.user_id, gr.nickname
		ORDER BY best_score DESC, best_at ASC
		LIMIT $2`, []interface{}{cutoff, limit}
	}
	return `SELECT MAX(gr.score_contribution) AS best_score,
	       COALESCE(NULLIF(gr.nickname, ''), gr.user_id::text) AS display_name,
	       MAX(gr.created_at) AS best_at
	FROM game_results gr
	JOIN game_sessions gs ON gs.id = gr.session_id
	WHERE gs.status = 'ended' AND gr.score_contribution > 0
	GROUP BY gr.user_id, gr.nickname
	ORDER BY best_score DESC, best_at ASC
	LIMIT $1`, []interface{}{limit}
}

func scanLeaderboardRows(rows pgx.Rows) ([]domain.LeaderboardEntry, error) {
	var entries []domain.LeaderboardEntry
	rank := 1
	for rows.Next() {
		var score int
		var displayName string
		var endedAt int64
		if scanErr := rows.Scan(&score, &displayName, &endedAt); scanErr != nil {
			return nil, fmt.Errorf("scan leaderboard row: %w", scanErr)
		}
		entries = append(entries, domain.LeaderboardEntry{
			Rank:    rank,
			Score:   score,
			Name:    displayName,
			EndedAt: endedAt,
		})
		rank++
	}
	return entries, rows.Err()
}
