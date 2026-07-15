package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/slogctx"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// UserRepository handles user persistence.
type UserRepository struct {
	baseRepository
}

// NewUserRepository creates a UserRepository.
func NewUserRepository(pool pgPool, deps ...Deps) *UserRepository {
	d := depsOrZero(deps...)
	return &UserRepository{baseRepository: newBaseRepository(pool, d)}
}

// CreateUser inserts a new user record into the database.
func (r *UserRepository) CreateUser(ctx context.Context, u *domain.User) error { //nolint:funlen // multi-step user creation with validation+outbox
	ctx, span := r.deps.Tracer.Start(ctx, "user_repo.CreateUser",
		trace.WithAttributes(attribute.String("db.system", "postgresql")),
	)
	defer span.End()

	emailHash, storedEmail, err := prepareEmailForStorage(u.Email)
	if err != nil {
		return err
	}

	outboxEmail, err := crypto.EncryptPIIForStorage(u.Email)
	if err != nil {
		return fmt.Errorf("encrypt email for outbox: %w", err)
	}
	// store-002: Encrypt nickname in outbox payload to prevent PII leakage
	// through the outbox_events table. Email was already encrypted; nickname
	// was the remaining plaintext PII field.
	outboxNickname, err := crypto.EncryptPIIForStorage(u.Nickname)
	if err != nil {
		return fmt.Errorf("encrypt nickname for outbox: %w", err)
	}
	outboxPayload, err := json.Marshal(map[string]interface{}{
		"event_type": "user.created",
		"user_id":    u.ID,
		"email":      outboxEmail,
		"nickname":   outboxNickname,
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
		if !errors.Is(err, domain.ErrDuplicateUser) {
			slogctx.LoggerFromContext(ctx).Error("create user failed",
				"error", err, "user_id", u.ID)
		}
		return err
	}

	logUserCreateAudit(ctx, u)
	return nil
}

// GetUserByEmail retrieves a user by email address.
func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	ctx, span := r.deps.Tracer.Start(ctx, "user_repo.GetUserByEmail",
		trace.WithAttributes(attribute.String("db.system", "postgresql")),
	)
	defer span.End()

	emailHash := crypto.EmailHMAC(email)
	var u *domain.User
	err := r.withRetryRead(ctx, func(ctx context.Context) error {
		// store-020: Wrap OR branch in parentheses so deleted_at IS NULL applies
		// to the entire WHERE clause. Previously AND bound tighter than OR,
		// causing soft-deleted users (email_hash match) to bypass the filter.
		row := r.pool.QueryRow(ctx,
			`SELECT id, email, nickname, palette, created_at, last_login FROM users WHERE (email_hash = $1 OR (email_hash IS NULL AND email = $2)) AND deleted_at IS NULL`,
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
		slogctx.LoggerFromContext(ctx).Error("get user by email failed", "error", err)
		return nil, err
	}
	return u, nil
}

// GetUserByID retrieves a user by ID.
func (r *UserRepository) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	ctx, span := r.deps.Tracer.Start(ctx, "user_repo.GetUserByID",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "SELECT"),
		),
	)
	defer span.End()

	var u *domain.User
	err := r.withRetryRead(ctx, func(ctx context.Context) error {
		row := r.pool.QueryRow(ctx,
			`SELECT id, email, nickname, palette, created_at, last_login FROM users WHERE id = $1 AND deleted_at IS NULL`, id)

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
		slogctx.LoggerFromContext(ctx).Error("get user by id failed",
			"error", err, "user_id", id)
		return nil, err
	}
	return u, nil
}

// UpdateUserLastLogin updates the last login timestamp for a user.
func (r *UserRepository) UpdateUserLastLogin(ctx context.Context, id string) error {
	ctx, span := r.deps.Tracer.Start(ctx, "user_repo.UpdateUserLastLogin",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "UPDATE"),
		),
	)
	defer span.End()

	return r.withRetryWrite(ctx, func(ctx context.Context) error {
		_, execErr := r.pool.Exec(ctx,
			`UPDATE users SET last_login = (EXTRACT(EPOCH FROM NOW()) * 1000)::bigint WHERE id = $1`, id)
		if execErr != nil {
			slogctx.LoggerFromContext(ctx).Error("update user last_login failed",
				"error", execErr, "user_id", id)
			return fmt.Errorf("update user last_login: %w", execErr)
		}
		return nil
	})
}

// AnonymizeUser irreversibly anonymizes a user's personal data for GDPR compliance.
func (r *UserRepository) AnonymizeUser(ctx context.Context, userID string) error {
	ctx, span := r.deps.Tracer.Start(ctx, "user_repo.AnonymizeUser",
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
	err = r.withRetryWrite(ctx, func(ctx context.Context) error {
		_, execErr := r.pool.Exec(ctx,
			`UPDATE users SET email = $1, email_hash = $2, nickname = 'Deleted User', deleted_at = $3, email_anonymized = true WHERE id = $4`,
			storedAnon, anonHash, now, userID)
		if execErr != nil {
			return fmt.Errorf("anonymize user: %w", execErr)
		}
		if _, outboxErr := r.pool.Exec(ctx,
			`UPDATE outbox_events SET payload = '{"anonymized":true}'::jsonb WHERE aggregate_type = 'user' AND aggregate_id = $1 AND processed_at IS NULL`,
			userID); outboxErr != nil {
			return fmt.Errorf("anonymize outbox: %w", outboxErr)
		}
		return nil
	})
	if err != nil {
		slogctx.LoggerFromContext(ctx).Error("anonymize user failed",
			"error", err, "user_id", userID)
		return err
	}
	return nil
}

// HardDeleteExpiredUsers permanently deletes anonymized users older than the retention period.
func (r *UserRepository) HardDeleteExpiredUsers(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		retentionDays = 30
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Unix()

	ctx, span := r.deps.Tracer.Start(ctx, "user_repo.HardDeleteExpiredUsers",
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

// GetGameResultsByUserID returns all game results for the given user.
func (r *UserRepository) GetGameResultsByUserID(ctx context.Context, userID string) ([]domain.GameResult, error) {
	ctx, span := r.deps.Tracer.Start(ctx, "user_repo.GetGameResultsByUserID",
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

// GetGameSessionsByUserID retrieves all game sessions created by the given user.
// auth-012: Required for complete GDPR data export.
func (r *UserRepository) GetGameSessionsByUserID(ctx context.Context, userID string) ([]domain.GameSession, error) {
	ctx, span := r.deps.Tracer.Start(ctx, "user_repo.GetGameSessionsByUserID",
		trace.WithAttributes(attribute.String("db.system", "postgresql")),
	)
	defer span.End()

	var sessions []domain.GameSession
	err := r.withRetryRead(ctx, func(ctx context.Context) error {
		rows, err := r.pool.Query(ctx,
			`SELECT id, lobby_code, created_by, status, started_at, ended_at, final_score
			 FROM game_sessions WHERE created_by = $1 ORDER BY COALESCE(started_at, 0) DESC`,
			userID,
		)
		if err != nil {
			return fmt.Errorf("query game sessions: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var gs domain.GameSession
			if err := rows.Scan(&gs.ID, &gs.LobbyCode, &gs.CreatedBy, &gs.Status, &gs.StartedAt, &gs.EndedAt, &gs.FinalScore); err != nil {
				return fmt.Errorf("scan game session: %w", err)
			}
			sessions = append(sessions, gs)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	if sessions == nil {
		sessions = []domain.GameSession{}
	}
	return sessions, nil
}

// ─── Email helpers ────────────────────────────────────────────────────

// prepareEmailForStorage returns HMAC hash and encrypted email for DB persistence.
func prepareEmailForStorage(email string) (hash, stored string, err error) {
	hash = crypto.EmailHMAC(email)
	stored, err = encryptEmailForStorageFn(email)
	if err != nil {
		return "", "", fmt.Errorf("encrypt email: %w", err)
	}
	return hash, stored, nil
}

// Test seam: encryptEmailForStorageFn is injectable for unit tests (e.g. simulate encryption failures).
var encryptEmailForStorageFn = crypto.EncryptPIIForStorage

// emailFromStorage decrypts a stored email value (legacy plaintext passes through).
func emailFromStorage(stored string) (string, error) {
	plain, err := crypto.DecryptEmailFromStorage(stored)
	if err != nil {
		return "", fmt.Errorf("decrypt email: %w", err)
	}
	return plain, nil
}

// ─── Audit ───────────────────────────────────────────────────────────

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
