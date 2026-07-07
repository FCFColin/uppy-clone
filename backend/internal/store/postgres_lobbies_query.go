package store

import (
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