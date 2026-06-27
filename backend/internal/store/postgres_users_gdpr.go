package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uppy-clone/backend/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// UpdateUserLastLogin sets last_login to the current unix timestamp.
// No retry: non-idempotent (updates timestamp).
func (s *PostgresStore) UpdateUserLastLogin(ctx context.Context, id string) error {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.UpdateUserLastLogin",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "UPDATE"),
		),
	)
	defer span.End()

	return s.withRetryWrite(ctx, func(ctx context.Context) error {
		_, execErr := s.pool.Exec(ctx,
			`UPDATE users SET last_login = EXTRACT(EPOCH FROM NOW())::bigint WHERE id = $1`, id)
		if execErr != nil {
			return fmt.Errorf("update user last_login: %w", execErr)
		}
		return nil
	})
}

// AnonymizeUser anonymizes a user's PII for GDPR Article 17 compliance.
// Sets email to deleted_<id>@anonymized, nickname to "Deleted User", marks deleted_at.
// The user row is retained (soft delete) for referential integrity with game results.
func (s *PostgresStore) AnonymizeUser(ctx context.Context, userID string) error {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.AnonymizeUser",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "UPDATE"),
			attribute.String("user.id", userID),
		),
	)
	defer span.End()

	now := time.Now().Unix()
	return s.withRetryWrite(ctx, func(ctx context.Context) error {
		_, execErr := s.pool.Exec(ctx,
			`UPDATE users SET email = $1, nickname = 'Deleted User', deleted_at = $2, email_anonymized = true WHERE id = $3`,
			"deleted_"+userID+"@anonymized", now, userID)
		if execErr != nil {
			return fmt.Errorf("anonymize user: %w", execErr)
		}
		return nil
	})
}

// HardDeleteExpiredUsers permanently removes users soft-deleted before the retention cutoff.
// Related game_results and game_sessions cascade via FK ON DELETE CASCADE.
func (s *PostgresStore) HardDeleteExpiredUsers(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		retentionDays = 30
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Unix()

	ctx, span := telemetry.Tracer().Start(ctx, "postgres.HardDeleteExpiredUsers",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "DELETE"),
			attribute.Int64("retention.cutoff_unix", cutoff),
		),
	)
	defer span.End()

	var deleted int64
	err := s.withRetryWrite(ctx, func(ctx context.Context) error {
		tag, execErr := s.pool.Exec(ctx,
			`DELETE FROM users WHERE deleted_at IS NOT NULL AND deleted_at < $1`, cutoff)
		if execErr != nil {
			return fmt.Errorf("hard delete expired users: %w", execErr)
		}
		deleted = tag.RowsAffected()
		return nil
	})
	return deleted, err
}
