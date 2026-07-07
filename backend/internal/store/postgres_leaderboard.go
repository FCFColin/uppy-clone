package store

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/uppy-clone/backend/internal/domain"
)

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
	defer rows.Close()
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