package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/sethvargo/go-retry"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/resilience"
	"github.com/uppy-clone/backend/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// SaveLobbyState upserts a lobby state record.
// Retry OK: UPSERT is idempotent.
func (s *PostgresStore) SaveLobbyState(ctx context.Context, ls *domain.LobbyState) error {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.SaveLobbyState",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("lobby.code", ls.Code),
		),
	)
	defer span.End()

	err := retry.Do(ctx, resilience.DefaultDBRetry(), func(ctx context.Context) error {
		_, cbErr := s.cb.Execute(func() (any, error) {
			_, execErr := s.pool.Exec(ctx,
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

// LoadLobbyState loads a lobby state by code. Returns nil if not found.
func (s *PostgresStore) LoadLobbyState(ctx context.Context, code string) (*domain.LobbyState, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.LoadLobbyState",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("lobby.code", code),
		),
	)
	defer span.End()

	var ls *domain.LobbyState
	err := s.withRetryRead(ctx, func(ctx context.Context) error {
		row := s.pool.QueryRow(ctx,
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

// DeleteLobbyState removes a lobby state by code.
// No retry: non-idempotent (deletion side effects).
func (s *PostgresStore) DeleteLobbyState(ctx context.Context, code string) error {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.DeleteLobbyState",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "DELETE"),
		),
	)
	defer span.End()

	return s.withRetryWrite(ctx, func(ctx context.Context) error {
		_, execErr := s.pool.Exec(ctx, `DELETE FROM lobby_states WHERE code = $1`, code)
		if execErr != nil {
			return fmt.Errorf("delete lobby state: %w", execErr)
		}
		return nil
	})
}
