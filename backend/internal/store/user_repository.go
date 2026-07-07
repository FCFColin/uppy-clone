package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// UserRepository handles user persistence.
type UserRepository struct {
	baseRepository
}

// NewUserRepository creates a UserRepository.
func NewUserRepository(pool pgPool) *UserRepository {
	return &UserRepository{baseRepository: newBaseRepository(pool)}
}

func (r *UserRepository) CreateUser(ctx context.Context, u *domain.User) error {
	ctx, span := telemetry.Tracer().Start(ctx, "user_repo.CreateUser",
		trace.WithAttributes(attribute.String("db.system", "postgresql")),
	)
	defer span.End()

	emailHash, storedEmail, err := prepareEmailForStorage(u.Email)
	if err != nil {
		return err
	}

	// email is intentionally included as plaintext in the outbox event payload.
	// This is a transactional outbox pattern: the outbox event is written in the same
	// DB transaction as the user insert, ensuring at-least-once delivery to downstream
	// consumers (e.g., welcome emails, analytics). The outbox consumer is responsible
	// for redacting or encrypting PII before forwarding to external systems.
	outboxPayload, err := json.Marshal(map[string]interface{}{
		"event_type": "user.created",
		"user_id":    u.ID,
		"email":      u.Email,
		"nickname":   u.Nickname,
		"created_at": u.CreatedAt,
	})
	if err != nil {
		return fmt.Errorf("marshal outbox payload: %w", err)
	}

	_, err = r.cb.Execute(func() (any, error) {
		tx, txErr := r.pool.Begin(ctx)
		if txErr != nil {
			return nil, fmt.Errorf("begin tx: %w", txErr)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		if _, execErr := tx.Exec(ctx,
			`INSERT INTO users (id, email, email_hash, nickname, palette, created_at, last_login) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			u.ID, storedEmail, emailHash, u.Nickname, u.Palette, u.CreatedAt, u.LastLogin); execErr != nil {
			var pgErr *pgconn.PgError
			if errors.As(execErr, &pgErr) && pgErr.Code == "23505" {
				return nil, domain.ErrDuplicateUser
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

func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "user_repo.GetUserByEmail",
		trace.WithAttributes(attribute.String("db.system", "postgresql")),
	)
	defer span.End()

	emailHash := crypto.EmailHMAC(email)
	var u *domain.User
	err := r.withRetryRead(ctx, func(ctx context.Context) error {
		row := r.pool.QueryRow(ctx,
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

func (r *UserRepository) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "user_repo.GetUserByID",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "SELECT"),
		),
	)
	defer span.End()

	var u *domain.User
	err := r.withRetryRead(ctx, func(ctx context.Context) error {
		row := r.pool.QueryRow(ctx,
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

func (r *UserRepository) UpdateUserLastLogin(ctx context.Context, id string) error {
	ctx, span := telemetry.Tracer().Start(ctx, "user_repo.UpdateUserLastLogin",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "UPDATE"),
		),
	)
	defer span.End()

	return r.withRetryWrite(ctx, func(ctx context.Context) error {
		_, execErr := r.pool.Exec(ctx,
			`UPDATE users SET last_login = EXTRACT(EPOCH FROM NOW())::bigint WHERE id = $1`, id)
		if execErr != nil {
			return fmt.Errorf("update user last_login: %w", execErr)
		}
		return nil
	})
}

func (r *UserRepository) AnonymizeUser(ctx context.Context, userID string) error {
	ctx, span := telemetry.Tracer().Start(ctx, "user_repo.AnonymizeUser",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "UPDATE"),
			attribute.String("user.id", userID),
		),
	)
	defer span.End()

	now := time.Now().Unix()
	anonEmail := "deleted_" + userID + "@anonymized"
	anonHash := crypto.EmailHMAC(anonEmail)
	storedAnon, err := encryptEmailForStorageFn(anonEmail)
	if err != nil {
		return fmt.Errorf("encrypt anonymized email: %w", err)
	}
	return r.withRetryWrite(ctx, func(ctx context.Context) error {
		_, execErr := r.pool.Exec(ctx,
			`UPDATE users SET email = $1, email_hash = $2, nickname = 'Deleted User', deleted_at = $3, email_anonymized = true WHERE id = $4`,
			storedAnon, anonHash, now, userID)
		if execErr != nil {
			return fmt.Errorf("anonymize user: %w", execErr)
		}
		return nil
	})
}

func (r *UserRepository) HardDeleteExpiredUsers(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		retentionDays = 30
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Unix()

	ctx, span := telemetry.Tracer().Start(ctx, "user_repo.HardDeleteExpiredUsers",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "DELETE"),
			attribute.Int64("retention.cutoff_unix", cutoff),
		),
	)
	defer span.End()

	var deleted int64
	err := r.withRetryWrite(ctx, func(ctx context.Context) error {
		tag, execErr := r.pool.Exec(ctx,
			`DELETE FROM users WHERE deleted_at IS NOT NULL AND deleted_at < $1`, cutoff)
		if execErr != nil {
			return fmt.Errorf("hard delete expired users: %w", execErr)
		}
		deleted = tag.RowsAffected()
		return nil
	})
	return deleted, err
}

func (r *UserRepository) GetGameResultsByUserID(ctx context.Context, userID string) ([]domain.GameResult, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "user_repo.GetGameResultsByUserID",
		trace.WithAttributes(attribute.String("db.system", "postgresql")),
	)
	defer span.End()

	var results []domain.GameResult
	err := r.withRetryRead(ctx, func(ctx context.Context) error {
		rows, err := r.pool.Query(ctx,
			`SELECT id, session_id, user_id, score_contribution, taps_count, created_at FROM game_results WHERE user_id = $1 ORDER BY created_at DESC LIMIT 100`, userID)
		if err != nil {
			return fmt.Errorf("query game results: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var r domain.GameResult
			if scanErr := rows.Scan(&r.ID, &r.SessionID, &r.UserID, &r.ScoreContribution, &r.TapsCount, &r.CreatedAt); scanErr != nil {
				return fmt.Errorf("scan game result: %w", scanErr)
			}
			results = append(results, r)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}
