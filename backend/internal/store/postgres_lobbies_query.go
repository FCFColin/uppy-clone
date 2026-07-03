package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/uppy-clone/backend/internal/domain"
)

func parseLobbyCursor(cursor string) (cursorUpdatedAt int64, cursorCode string) {
	if cursor == "" {
		return 0, ""
	}
	parts := strings.SplitN(cursor, "|", 2)
	if len(parts) == 2 {
		if v, err := fmt.Sscanf(parts[0], "%d", &cursorUpdatedAt); err != nil || v != 1 {
			cursorUpdatedAt = 0
		}
		cursorCode = parts[1]
	}
	return cursorUpdatedAt, cursorCode
}

func (s *PostgresStore) countAllLobbies(ctx context.Context) (int, error) {
	var total int
	err := s.withRetryRead(ctx, func(ctx context.Context) error {
		if countErr := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM lobby_states`).Scan(&total); countErr != nil {
			return fmt.Errorf("count lobbies: %w", countErr)
		}
		return nil
	})
	return total, err
}

func (s *PostgresStore) fetchLobbiesPage(ctx context.Context, fetchLimit int, cursorUpdatedAt int64, cursorCode string) ([]domain.LobbyState, error) {
	var lobbies []domain.LobbyState
	err := s.withRetryRead(ctx, func(ctx context.Context) error {
		rows, queryErr := s.queryLobbies(ctx, fetchLimit, cursorUpdatedAt, cursorCode)
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

func (s *PostgresStore) queryLobbies(ctx context.Context, fetchLimit int, cursorUpdatedAt int64, cursorCode string) (pgx.Rows, error) {
	if cursorUpdatedAt > 0 {
		return s.pool.Query(ctx,
			`SELECT id, code, state, updated_at, created_at FROM lobby_states
			 WHERE (updated_at, code) < ($1, $2)
			 ORDER BY updated_at DESC, code DESC LIMIT $3`,
			cursorUpdatedAt, cursorCode, fetchLimit)
	}
	return s.pool.Query(ctx,
		`SELECT id, code, state, updated_at, created_at FROM lobby_states
		 ORDER BY updated_at DESC, code DESC LIMIT $1`,
		fetchLimit)
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
