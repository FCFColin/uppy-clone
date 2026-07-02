package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// CreateUser inserts a new user record and enqueues a user.created outbox event
// in the same ACID transaction.
// No retry: non-idempotent (would create duplicates).
func (s *PostgresStore) CreateUser(ctx context.Context, u *domain.User) error {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.CreateUser",
		trace.WithAttributes(attribute.String("db.system", "postgresql")),
	)
	defer span.End()

	emailHash, storedEmail, err := prepareEmailForStorage(u.Email)
	if err != nil {
		return err
	}

	outboxPayload, err := jsonMarshalFn(map[string]interface{}{
		"event_type": "user.created",
		"user_id":    u.ID,
		"email":      u.Email,
		"nickname":   u.Nickname,
		"created_at": u.CreatedAt,
	})
	if err != nil {
		return fmt.Errorf("marshal outbox payload: %w", err)
	}

	_, err = s.cb.Execute(func() (any, error) {
		tx, txErr := s.pool.Begin(ctx)
		if txErr != nil {
			return nil, fmt.Errorf("begin tx: %w", txErr)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		if _, execErr := tx.Exec(ctx,
			`INSERT INTO users (id, email, email_hash, nickname, palette, created_at, last_login) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			u.ID, storedEmail, emailHash, u.Nickname, u.Palette, u.CreatedAt, u.LastLogin); execErr != nil {
			var pgErr *pgconn.PgError
			if errors.As(execErr, &pgErr) && pgErr.Code == "23505" {
				return nil, ErrDuplicateUser
			}
			return nil, fmt.Errorf("create user: %w", execErr)
		}

		if _, execErr := tx.Exec(ctx,
			`INSERT INTO outbox_events (aggregate_type, aggregate_id, payload) VALUES ($1, $2, $3)`,
			"user", u.ID, outboxPayload); execErr != nil {
			return nil, fmt.Errorf("insert outbox event: %w", execErr)
		}

		if commitErr := tx.Commit(ctx); commitErr != nil {
			return nil, fmt.Errorf("commit create user: %w", commitErr)
		}
		return nil, nil
	})
	if err != nil {
		return err
	}

	logUserCreateAudit(ctx, u)
	return nil
}

var jsonMarshalFn = json.Marshal

func logUserCreateAudit(ctx context.Context, u *domain.User) {
	audit.Log(ctx, audit.AuditEntry{
		Action:   "user.create",
		ActorID:  u.ID,
		Resource: "user/" + u.ID,
		After: map[string]interface{}{
			"id":       u.ID,
			"nickname": u.Nickname,
		},
	})
}
