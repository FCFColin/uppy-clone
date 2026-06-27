package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// GetUserByEmail returns a user by email. Returns nil if not found.
func (s *PostgresStore) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.GetUserByEmail",
		trace.WithAttributes(attribute.String("db.system", "postgresql")),
	)
	defer span.End()

	emailHash := crypto.EmailHMAC(email)

	var u *domain.User
	err := s.withRetryRead(ctx, func(ctx context.Context) error {
		row := s.pool.QueryRow(ctx,
			`SELECT id, email, nickname, palette, created_at, last_login FROM users WHERE email_hash = $1 OR (email_hash IS NULL AND email = $2)`,
			emailHash, email)

		var user domain.User
		if scanErr := row.Scan(&user.ID, &user.Email, &user.Nickname, &user.Palette, &user.CreatedAt, &user.LastLogin); scanErr != nil {
			if errors.Is(scanErr, pgx.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("get user by email: %w", scanErr)
		}
		plain, decErr := emailFromStorage(user.Email)
		if decErr != nil {
			return decErr
		}
		user.Email = plain
		u = &user
		return nil
	})
	if err != nil {
		return nil, err
	}
	return u, nil
}

// GetUserByID returns a user by ID. Returns nil if not found.
func (s *PostgresStore) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.GetUserByID",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "SELECT"),
		),
	)
	defer span.End()

	var u *domain.User
	err := s.withRetryRead(ctx, func(ctx context.Context) error {
		row := s.pool.QueryRow(ctx,
			`SELECT id, email, nickname, palette, created_at, last_login FROM users WHERE id = $1`, id)

		var user domain.User
		if scanErr := row.Scan(&user.ID, &user.Email, &user.Nickname, &user.Palette, &user.CreatedAt, &user.LastLogin); scanErr != nil {
			if errors.Is(scanErr, pgx.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("get user by id: %w", scanErr)
		}
		plain, decErr := emailFromStorage(user.Email)
		if decErr != nil {
			return decErr
		}
		user.Email = plain
		u = &user
		return nil
	})
	if err != nil {
		return nil, err
	}
	return u, nil
}
