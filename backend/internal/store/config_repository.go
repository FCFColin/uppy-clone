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

// ConfigRepository handles admin config persistence.
type ConfigRepository struct {
	baseRepository
}

// NewConfigRepository creates a ConfigRepository.
func NewConfigRepository(pool *pgxpool.Pool) *ConfigRepository {
	return &ConfigRepository{baseRepository: newBaseRepository(pool)}
}

func (r *ConfigRepository) GetConfig(ctx context.Context, id string) (*domain.AppConfig, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "config_repo.GetConfig",
		trace.WithAttributes(attribute.String("db.system", "postgresql")),
	)
	defer span.End()

	var c *domain.AppConfig
	err := r.withRetryRead(ctx, func(ctx context.Context) error {
		row := r.pool.QueryRow(ctx, `SELECT id, config, updated_at FROM admin_config WHERE id = $1`, id)

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

func (r *ConfigRepository) SaveConfig(ctx context.Context, c *domain.AppConfig) error {
	ctx, span := telemetry.Tracer().Start(ctx, "config_repo.SaveConfig",
		trace.WithAttributes(attribute.String("db.system", "postgresql")),
	)
	defer span.End()

	err := retry.Do(ctx, resilience.DefaultDBRetry(), func(ctx context.Context) error {
		_, cbErr := r.cb.Execute(func() (any, error) {
			_, execErr := r.pool.Exec(ctx,
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
