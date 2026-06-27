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

// GetConfig loads an admin config by ID. Returns nil if not found.
func (s *PostgresStore) GetConfig(ctx context.Context, id string) (*domain.AppConfig, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.GetConfig",
		trace.WithAttributes(attribute.String("db.system", "postgresql")),
	)
	defer span.End()

	var c *domain.AppConfig
	err := s.withRetryRead(ctx, func(ctx context.Context) error {
		row := s.pool.QueryRow(ctx, `SELECT id, config, updated_at FROM admin_config WHERE id = $1`, id)

		var cfg domain.AppConfig
		if scanErr := row.Scan(&cfg.ID, &cfg.Config, &cfg.UpdatedAt); scanErr != nil {
			if errors.Is(scanErr, pgx.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("get config: %w", scanErr)
		}
		c = &cfg
		return nil
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

// SaveConfig upserts an admin config record.
func (s *PostgresStore) SaveConfig(ctx context.Context, c *domain.AppConfig) error {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.SaveConfig",
		trace.WithAttributes(attribute.String("db.system", "postgresql")),
	)
	defer span.End()

	err := retry.Do(ctx, resilience.DefaultDBRetry(), func(ctx context.Context) error {
		_, cbErr := s.cb.Execute(func() (any, error) {
			_, execErr := s.pool.Exec(ctx,
				`INSERT INTO admin_config (id, config, updated_at) VALUES ($1, $2, $3)
				 ON CONFLICT (id) DO UPDATE SET config = EXCLUDED.config, updated_at = EXCLUDED.updated_at`,
				c.ID, c.Config, c.UpdatedAt)
			if execErr != nil {
				return nil, fmt.Errorf("save config: %w", execErr)
			}
			return nil, nil
		})
		return resilience.MaybeRetryable(cbErr)
	})
	return err
}
