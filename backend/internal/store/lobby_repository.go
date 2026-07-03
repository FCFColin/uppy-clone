package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sethvargo/go-retry"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/resilience"
	"github.com/uppy-clone/backend/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// LobbyRepository handles lobby state persistence.
type LobbyRepository struct {
	baseRepository
}

// NewLobbyRepository creates a LobbyRepository.
func NewLobbyRepository(pool *pgxpool.Pool) *LobbyRepository {
	return &LobbyRepository{baseRepository: newBaseRepository(pool)}
}

func (r *LobbyRepository) SaveLobbyState(ctx context.Context, ls *domain.LobbyState) error {
	ctx, span := telemetry.Tracer().Start(ctx, "lobby_repo.SaveLobbyState",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("lobby.code", ls.Code),
		),
	)
	defer span.End()

	err := retry.Do(ctx, resilience.DefaultDBRetry(), func(ctx context.Context) error {
		_, cbErr := r.cb.Execute(func() (any, error) {
			_, execErr := r.pool.Exec(ctx,
				`INSERT INTO lobby_states (id, code, state, updated_at, created_at) VALUES ($1, $2, $3, $4, $5)
				 ON CONFLICT (code) DO UPDATE SET state = EXCLUDED.state, updated_at = EXCLUDED.updated_at`,
				ls.ID, ls.Code, ls.State, ls.UpdatedAt, ls.CreatedAt)
			if execErr != nil {
				return nil, fmt.Errorf("save lobby state: %w", execErr)
			}
			return nil, nil
		})
		return resilience.MaybeRetryable(cbErr)
	})
	return err
}

func (r *LobbyRepository) LoadLobbyState(ctx context.Context, code string) (*domain.LobbyState, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "lobby_repo.LoadLobbyState",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("lobby.code", code),
		),
	)
	defer span.End()

	var ls *domain.LobbyState
	err := r.withRetryRead(ctx, func(ctx context.Context) error {
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
		return nil, err
	}
	return ls, nil
}

func (r *LobbyRepository) DeleteLobbyState(ctx context.Context, code string) error {
	ctx, span := telemetry.Tracer().Start(ctx, "lobby_repo.DeleteLobbyState",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "DELETE"),
		),
	)
	defer span.End()

	return r.withRetryWrite(ctx, func(ctx context.Context) error {
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
		attribute.String("db.system", "postgresql"),
		attribute.Int("db.limit", limit),
	}
	if cursor != "" {
		spanAttrs = append(spanAttrs, attribute.String("db.cursor", cursor))
	}

	ctx, span := telemetry.Tracer().Start(ctx, "lobby_repo.LoadAllActiveLobbies",
		trace.WithAttributes(spanAttrs...),
	)
	defer span.End()

	cursorUpdatedAt, cursorCode := parseLobbyCursor(cursor)

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

func (r *LobbyRepository) countAllLobbies(ctx context.Context) (int, error) {
	var total int
	err := r.withRetryRead(ctx, func(ctx context.Context) error {
		if countErr := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM lobby_states`).Scan(&total); countErr != nil {
			return fmt.Errorf("count lobbies: %w", countErr)
		}
		return nil
	})
	return total, err
}

func (r *LobbyRepository) fetchLobbiesPage(ctx context.Context, fetchLimit int, cursorUpdatedAt int64, cursorCode string) ([]domain.LobbyState, error) {
	var lobbies []domain.LobbyState
	err := r.withRetryRead(ctx, func(ctx context.Context) error {
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
