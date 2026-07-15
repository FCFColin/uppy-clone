package store

import (
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/uppy-clone/backend/internal/domain"
)

// parseLobbyCursor parses a pagination cursor of the form "<unix_millis>|<code>".
// Returns an error if the cursor is non-empty but malformed (store-013).
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
