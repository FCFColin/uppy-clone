package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/util"
	"go.opentelemetry.io/otel/attribute"
)

// LobbyRepository handles lobby state persistence.
type LobbyRepository struct {
	baseRepository
}

// NewLobbyRepository creates a LobbyRepository.
func NewLobbyRepository(pool pgPool, deps ...Deps) *LobbyRepository {
	d := depsOrZero(deps...)
	return &LobbyRepository{baseRepository: newBaseRepository(pool, d)}
}

// SaveLobbyState inserts or updates a lobby state by code (upsert).
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

// LoadLobbyState retrieves a lobby state by its code.
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

// DeleteLobbyState removes a lobby state by its code.
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

// LoadAllActiveLobbies returns a paginated list of active lobby states.
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
