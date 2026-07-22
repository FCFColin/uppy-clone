package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/uppy-clone/backend/internal/domain"
)

// UnmarshalConfig parses the JSON config column into the target struct.
func UnmarshalConfig(raw string, target interface{}) error {
	return json.Unmarshal([]byte(raw), target)
}

// ConfigRepository handles admin config persistence.
type ConfigRepository struct {
	baseRepository
}

// NewConfigRepository creates a ConfigRepository.
func NewConfigRepository(pool pgPool, deps ...Deps) *ConfigRepository {
	d := depsOrZero(deps...)
	return &ConfigRepository{baseRepository: newBaseRepository(pool, d)}
}

// GetConfig retrieves an application configuration by ID.
func (r *ConfigRepository) GetConfig(ctx context.Context, id string) (*domain.AppConfig, error) {
	ctx, span := withSpan(ctx, r.deps.Tracer, "config_repo.GetConfig")
	defer span.End()

	var c *domain.AppConfig
	err := r.withRetry(ctx, func(ctx context.Context) error {
		row := r.pool.QueryRow(ctx, `SELECT id, config, updated_at FROM admin_config WHERE id = $1`, id)

		var cfg domain.AppConfig
		if scanErr := row.Scan(&cfg.ID, &cfg.Config, &cfg.UpdatedAt); scanErr != nil {
			if errors.Is(scanErr, pgx.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("get config: %w", scanErr)
		}
		// store-018: Parse the JSON config to populate EmailEnabled and EmailFrom
		// fields that were always zero because they are part of the JSON config column.
		var parsed struct {
			EmailEnabled bool   `json:"email_enabled"`
			EmailFrom    string `json:"email_from"`
		}
		if jsonErr := UnmarshalConfig(cfg.Config, &parsed); jsonErr == nil {
			cfg.EmailEnabled = parsed.EmailEnabled
			cfg.EmailFrom = parsed.EmailFrom
		}
		c = &cfg
		return nil
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

// SaveConfig persists an application configuration.
func (r *ConfigRepository) SaveConfig(ctx context.Context, c *domain.AppConfig) error {
	ctx, span := withSpan(ctx, r.deps.Tracer, "config_repo.SaveConfig")
	defer span.End()

	err := r.withRetry(ctx, func(ctx context.Context) error {
		_, execErr := r.pool.Exec(ctx,
			`INSERT INTO admin_config (id, config, updated_at) VALUES ($1, $2, $3)
			 ON CONFLICT (id) DO UPDATE SET config = EXCLUDED.config, updated_at = EXCLUDED.updated_at`,
			c.ID, c.Config, c.UpdatedAt)
		if execErr != nil {
			return fmt.Errorf("save config: %w", execErr)
		}
		return nil
	})
	return err
}
