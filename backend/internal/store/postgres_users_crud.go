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

// T40 [G-5] PostgreSQL 邮箱加密 — 延期实施说明（DEFERRED）
//
// 现状：users.email 当前以明文存储，存在数据库泄露时 PII 暴露风险。
//
// 为何不能直接加密 email 列：
//   crypto.Encrypt 使用 AES-256-GCM + 随机 nonce（非确定性加密），同一明文每次加密
//   产生不同密文。因此无法用 `WHERE email = $1` 查询加密后的邮箱——加密后的查询值
//   永远不会匹配数据库中的密文。GetUserByEmail 用于 magic link 登录流程
//   （auth/magiclink.go:191），查询能力是认证链路的硬性依赖。
//
// 正确方案（需 Schema 迁移，故延期）：
//   1. 新增 email_hash 列（HMAC-SHA256(email)），建立唯一索引，用于等值查询
//   2. email 列改为存储 AES-256-GCM 密文（保留非确定性加密的安全性）
//   3. GetUserByEmail 改为 `WHERE email_hash = $1` 查询，取出后 crypto.Decrypt 解密
//   4. CreateUser 同时写入 email_hash 和加密后的 email
//   5. AnonymizeUser 同步更新 email_hash（GDPR 匿名化）
//
// 此任务标记为 DEFERRED，待 schema migration 窗口期实施。

// CreateUser inserts a new user record and enqueues a user.created outbox event
// in the same ACID transaction.
// No retry: non-idempotent (would create duplicates).
func (s *PostgresStore) CreateUser(ctx context.Context, u *domain.User) error {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.CreateUser",
		trace.WithAttributes(attribute.String("db.system", "postgresql")),
	)
	defer span.End()

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

	_, err = s.cb.Execute(func() (any, error) {
		tx, txErr := s.pool.Begin(ctx)
		if txErr != nil {
			return nil, fmt.Errorf("begin tx: %w", txErr)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		if _, execErr := tx.Exec(ctx,
			`INSERT INTO users (id, email, nickname, palette, created_at, last_login) VALUES ($1, $2, $3, $4, $5, $6)`,
			u.ID, u.Email, u.Nickname, u.Palette, u.CreatedAt, u.LastLogin); execErr != nil {
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

	audit.Log(ctx, audit.AuditEntry{
		Action:   "user.create",
		ActorID:  u.ID,
		Resource: "user/" + u.ID,
		After: map[string]interface{}{
			"id":       u.ID,
			"nickname": u.Nickname,
		},
	})

	return nil
}
