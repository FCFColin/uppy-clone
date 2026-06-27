package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uppy-clone/backend/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// LeaderboardEntry is a single row on the public leaderboard.
type LeaderboardEntry struct {
	Rank      int    `json:"rank"`
	Score     int    `json:"score"`
	LobbyCode string `json:"lobbyCode"`
	EndedAt   int64  `json:"endedAt"`
}

// GetLeaderboard returns top game sessions by final team score.
func (s *PostgresStore) GetLeaderboard(ctx context.Context, scope string, limit int) ([]LeaderboardEntry, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.GetLeaderboard",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("leaderboard.scope", scope),
		),
	)
	defer span.End()

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var entries []LeaderboardEntry
	err := s.withRetryRead(ctx, func(ctx context.Context) error {
		var query string
		var args []interface{}
		if scope == "weekly" {
			cutoff := time.Now().Add(-7 * 24 * time.Hour).UnixMilli()
			query = `SELECT final_score, lobby_code, ended_at
				FROM game_sessions
				WHERE status = 'ended' AND final_score > 0 AND ended_at >= $1
				ORDER BY final_score DESC, ended_at ASC
				LIMIT $2`
			args = []interface{}{cutoff, limit}
		} else {
			query = `SELECT final_score, lobby_code, ended_at
				FROM game_sessions
				WHERE status = 'ended' AND final_score > 0
				ORDER BY final_score DESC, ended_at ASC
				LIMIT $1`
			args = []interface{}{limit}
		}

		rows, err := s.pool.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("query leaderboard: %w", err)
		}
		defer rows.Close()

		rank := 1
		for rows.Next() {
			var score int
			var lobbyCode string
			var endedAt int64
			if scanErr := rows.Scan(&score, &lobbyCode, &endedAt); scanErr != nil {
				return fmt.Errorf("scan leaderboard row: %w", scanErr)
			}
			entries = append(entries, LeaderboardEntry{
				Rank:      rank,
				Score:     score,
				LobbyCode: lobbyCode,
				EndedAt:   endedAt,
			})
			rank++
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []LeaderboardEntry{}
	}
	return entries, nil
}

// GetUserBestScore returns the highest score contribution for a user.
func (s *PostgresStore) GetUserBestScore(ctx context.Context, userID string) (int, int, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.GetUserBestScore",
		trace.WithAttributes(attribute.String("db.system", "postgresql")),
	)
	defer span.End()

	var bestScore int
	var gamesPlayed int
	err := s.withRetryRead(ctx, func(ctx context.Context) error {
		return s.pool.QueryRow(ctx,
			`SELECT COALESCE(MAX(score_contribution), 0), COUNT(*)
			 FROM game_results WHERE user_id = $1`,
			userID,
		).Scan(&bestScore, &gamesPlayed)
	})
	return bestScore, gamesPlayed, err
}
