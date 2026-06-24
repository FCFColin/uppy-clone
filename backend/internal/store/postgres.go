package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sethvargo/go-retry"
	"github.com/sony/gobreaker/v2"
	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/resilience"
	"github.com/uppy-clone/backend/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/golang-migrate/migrate/v4"
	// imported for side-effect: registers postgres driver for migrate
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// ErrDuplicateUser 表示创建用户时发生唯一约束冲突（duplicate key）。
// 企业为何需要：调用方需要区分"重复键"与其他数据库错误以做幂等处理，
// 哨兵错误让 errors.Is 精确匹配，避免脆弱的字符串包含判断。
var ErrDuplicateUser = errors.New("duplicate user")

// withRetryRead wraps a read-only operation with retry and circuit breaker.
// 企业为何需要：读操作幂等可安全重试，统一封装消除 15+ 处 retry+cb 嵌套样板代码。
// 关键：回调必须用 resilience.MaybeRetryable 包装错误，否则 sethvargo/go-retry
// 会将普通 error 视为不可重试而立即返回，配置的重试次数形同虚设。
func (s *PostgresStore) withRetryRead(ctx context.Context, fn func(context.Context) error) error {
	return retry.Do(ctx, resilience.DefaultDBRetry(), func(ctx context.Context) error {
		_, err := s.cb.Execute(func() (any, error) {
			return nil, fn(ctx)
		})
		return resilience.MaybeRetryable(err)
	})
}

// withRetryWrite wraps a write operation with circuit breaker (no retry for writes).
// 企业为何需要：写操作非幂等不可重试，仅用熔断器防止级联故障。
func (s *PostgresStore) withRetryWrite(ctx context.Context, fn func(context.Context) error) error {
	_, err := s.cb.Execute(func() (any, error) {
		return nil, fn(ctx)
	})
	return err
}

// PostgresStore provides PostgreSQL-backed persistence.
type PostgresStore struct {
	pool *pgxpool.Pool
	cb   *gobreaker.CircuitBreaker[any]

	// lastAcquireDuration/lastAcquireCount track previous pool stat samples
	// for delta-based DBPoolAcquireDuration observation.
	lastAcquireDuration atomic.Value // float64
	lastAcquireCount    atomic.Value // int64
}

// NewPostgresStore creates a connection pool and validates connectivity.
func NewPostgresStore(connString string, timeouts config.TimeoutConfig) (*PostgresStore, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeouts.PGConnectTimeout)
	defer cancel()

	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Enterprise rationale: Default pool config is unbounded which can exhaust
	// PostgreSQL connections under load. Values are env-configurable so different
	// environments (dev/staging/prod) can tune without redeploying. Defaults:
	// - MaxConns: 25 (PostgreSQL default max_connections=100, allow headroom)
	// - MinConns: 5 (keep warm connections to avoid cold-start latency)
	// - MaxConnLifetime: 30m (prevent long-lived connections from stale routing)
	// - MaxConnIdleTime: 5m (reclaim idle connections)
	// - HealthCheckPeriod: 30s (detect dead connections promptly)
	// Trade-off: More connections = more memory per connection (~10MB each in PG).
	config.MaxConns = int32(getEnvInt("PG_POOL_MAX_CONNS", 25)) //nolint:gosec // config value, reasonable range
	config.MinConns = int32(getEnvInt("PG_POOL_MIN_CONNS", 5))  //nolint:gosec // config value, reasonable range
	config.MaxConnLifetime = getEnvDuration("PG_POOL_MAX_CONN_LIFETIME", 30*time.Minute)
	config.MaxConnIdleTime = getEnvDuration("PG_POOL_MAX_CONN_IDLE_TIME", 5*time.Minute)
	config.HealthCheckPeriod = getEnvDuration("PG_POOL_HEALTH_CHECK_PERIOD", 30*time.Second)

	// Instrument connection acquires via PrepareConn callback.
	// Enterprise rationale: Pool acquire count is a Golden Signal for saturation.
	// Spikes in acquire rate indicate increased load; sustained high rate + high
	// acquire duration signals pool exhaustion.
	// DBPoolAcquireDuration is observed periodically via ObservePoolStats() using
	// pool.Stat().AcquireDuration() delta, since pgxpool does not provide
	// per-acquire duration callbacks.
	config.PrepareConn = func(_ context.Context, conn *pgx.Conn) (bool, error) {
		metrics.DBPoolAcquireCount.Inc()
		return true, nil
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	return &PostgresStore{
		pool: pool,
		cb:   resilience.NewPostgresBreaker(),
	}, nil
}

// Close releases the connection pool.
func (s *PostgresStore) Close() {
	s.pool.Close()
}

// getEnvDuration returns the environment variable value as time.Duration, or a default.
// Invalid or non-positive values fall back to defaultVal. Accepts Go duration
// strings (e.g., "30m", "5s", "1h").
// 企业为何需要：连接池生命周期参数需按环境调优，硬编码需重新部署才能调整。
func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return defaultVal
}

// Pool returns the underlying connection pool for health checks and metrics.
func (s *PostgresStore) Pool() *pgxpool.Pool {
	return s.pool
}

// PoolStats returns the current connection pool statistics.
// Enterprise rationale: Pool saturation is a Golden Signal — when all
// connections are in use, new requests queue and latency spikes.
func (s *PostgresStore) PoolStats() *pgxpool.Stat {
	return s.pool.Stat()
}

// ObservePoolStats records pool acquire duration delta into the
// DBPoolAcquireDuration histogram. Call periodically (e.g., every 15s)
// from a background goroutine.
//
// 企业为何需要：pgxpool 不提供 per-acquire 回调，但 pool.Stat().AcquireDuration()
// 返回累计等待时间。周期性采样 delta 可观测获取耗时趋势——池接近耗尽时
// 获取耗时先于错误率上升，提供早期预警。
func (s *PostgresStore) ObservePoolStats() {
	stat := s.pool.Stat()
	// Observe idle/in-use gauges (already done elsewhere, but safe to repeat)
	metrics.DBPoolIdleConns.Set(float64(stat.IdleConns()))
	metrics.DBPoolInUseConns.Set(float64(stat.AcquiredConns()))
	// Observe acquire duration delta if we have a previous sample
	currentDuration := stat.AcquireDuration().Seconds()
	currentCount := stat.AcquireCount()
	if prevDur, ok := s.lastAcquireDuration.Load().(float64); ok && prevDur > 0 {
		if prevCnt, ok := s.lastAcquireCount.Load().(int64); ok && currentCount > prevCnt {
			delta := currentDuration - prevDur
			countDelta := float64(currentCount - prevCnt)
			if delta > 0 && countDelta > 0 {
				metrics.DBPoolAcquireDuration.Observe(delta / countDelta)
			}
		}
	}
	s.lastAcquireDuration.Store(currentDuration)
	s.lastAcquireCount.Store(currentCount)
}

// RunMigrations applies all pending migrations from the given directory.
func (s *PostgresStore) RunMigrations(migrationsPath string) error {
	m, err := migrate.New("file://"+migrationsPath, s.pool.Config().ConnString())
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

// --- User CRUD ---

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
//      （HMAC 而非裸 SHA256，防止彩虹表攻击；HMAC 密钥与加密密钥分离）
//   2. email 列改为存储 AES-256-GCM 密文（保留非确定性加密的安全性）
//   3. GetUserByEmail 改为 `WHERE email_hash = $1` 查询，取出后 crypto.Decrypt 解密
//   4. CreateUser 同时写入 email_hash 和加密后的 email
//   5. AnonymizeUser 同步更新 email_hash（GDPR 匿名化）
//
// 数据迁移步骤（不在本次实施范围）：
//   a. 创建 migration: ALTER TABLE users ADD COLUMN email_hash VARCHAR(64);
//   b. 回填脚本: 对每行 users 计算 HMAC-SHA256(email) 写入 email_hash
//   c. 回填脚本: 对每行 users 用 crypto.Encrypt 加密 email 并更新
//   d. 创建唯一索引: CREATE UNIQUE INDEX idx_users_email_hash ON users(email_hash);
//   e. 修改应用层代码（GetUserByEmail/CreateUser/AnonymizeUser）
//   f. 可选: 删除旧明文 email 列或保留作为兼容期
//
// 风险：迁移期间需双写（同时写明文和密文），确保回滚安全。
// 此任务标记为 DEFERRED，待 schema migration 窗口期实施。

// GetUserByEmail returns a user by email. Returns nil if not found.
func (s *PostgresStore) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.GetUserByEmail",
		trace.WithAttributes(attribute.String("db.system", "postgresql")),
	)
	defer span.End()

	var u *domain.User
	err := s.withRetryRead(ctx, func(ctx context.Context) error {
		row := s.pool.QueryRow(ctx,
			`SELECT id, email, nickname, palette, created_at, last_login FROM users WHERE email = $1`, email)

		var user domain.User
		if scanErr := row.Scan(&user.ID, &user.Email, &user.Nickname, &user.Palette, &user.CreatedAt, &user.LastLogin); scanErr != nil {
			if errors.Is(scanErr, pgx.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("get user by email: %w", scanErr)
		}
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
			attribute.String("db.statement", "SELECT id, email, nickname, palette, created_at, last_login FROM users WHERE id = $1"),
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
		u = &user
		return nil
	})
	if err != nil {
		return nil, err
	}
	return u, nil
}

// CreateUser inserts a new user record and enqueues a user.created outbox event
// in the same ACID transaction.
// 企业为何需要：Transactional Outbox 模式保证业务数据与事件原子性提交。若 PG 写入成功但 Redis 发布失败，
// 事件不会丢失——后台 Publisher 轮询未处理记录重新发布（at-least-once 语义）。
// No retry: non-idempotent (would create duplicates).
func (s *PostgresStore) CreateUser(ctx context.Context, u *domain.User) error {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.CreateUser",
		trace.WithAttributes(attribute.String("db.system", "postgresql")),
	)
	defer span.End()

	// Construct outbox event payload (user.created)
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

		// 1. Insert user
		if _, execErr := tx.Exec(ctx,
			`INSERT INTO users (id, email, nickname, palette, created_at, last_login) VALUES ($1, $2, $3, $4, $5, $6)`,
			u.ID, u.Email, u.Nickname, u.Palette, u.CreatedAt, u.LastLogin); execErr != nil {
			var pgErr *pgconn.PgError
			if errors.As(execErr, &pgErr) && pgErr.Code == "23505" {
				return nil, ErrDuplicateUser
			}
			return nil, fmt.Errorf("create user: %w", execErr)
		}

		// 2. Insert outbox event in the same transaction
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

	// Audit: user creation
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

// UpdateUserLastLogin sets last_login to the current unix timestamp.
// No retry: non-idempotent (updates timestamp).
func (s *PostgresStore) UpdateUserLastLogin(ctx context.Context, id string) error {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.UpdateUserLastLogin",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "UPDATE"),
			attribute.String("db.statement", "UPDATE users SET last_login = EXTRACT(EPOCH FROM NOW())::bigint WHERE id = $1"),
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
// The user row is retained (soft delete) for referential integrity with game results;
// a scheduled cleanup job hard-deletes rows after the retention period.
// 企业为何需要：GDPR 第 17 条要求删除用户 PII。立即匿名化满足合规要求，
// 同时保留用户行避免外键约束违反（游戏结果引用 users.id）。
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

// --- Game Session ---

// CreateGameSession inserts a new game session record.
// No retry: non-idempotent.
func (s *PostgresStore) CreateGameSession(ctx context.Context, gs *domain.GameSession) error {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.CreateGameSession",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "INSERT"),
			attribute.String("db.statement", "INSERT INTO game_sessions (id, lobby_code, created_by, status, started_at, ended_at, final_score) VALUES ($1, $2, $3, $4, $5, $6, $7)"),
		),
	)
	defer span.End()

	return s.withRetryWrite(ctx, func(ctx context.Context) error {
		_, execErr := s.pool.Exec(ctx,
			`INSERT INTO game_sessions (id, lobby_code, created_by, status, started_at, ended_at, final_score) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			gs.ID, gs.LobbyCode, gs.CreatedBy, gs.Status, gs.StartedAt, gs.EndedAt, gs.FinalScore)
		if execErr != nil {
			return fmt.Errorf("create game session: %w", execErr)
		}
		return nil
	})
}

// --- Lobby State ---

// EndGameAndRecordResults ends a game session and records all player results
// in a single ACID transaction.
//
// 企业为何需要：事务边界错误产生孤儿数据。游戏结束但结果未记录会导致数据不一致。ACID 事务是数据库可靠性的基础。
//
// Without this, ending the session and recording results would be two separate operations.
// If the process crashes between them, the session is marked 'ended' but no
// results are recorded — producing orphan data that breaks leaderboards and
// player statistics. A single transaction ensures atomicity: either both
// succeed or neither does.
func (s *PostgresStore) EndGameAndRecordResults(ctx context.Context, sessionID string, endedAt int64, finalScore int, results []domain.GameResult) error {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.EndGameAndRecordResults",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.session_id", sessionID),
			attribute.Int("db.results_count", len(results)),
		),
	)
	defer span.End()

	// No retry: non-idempotent (updates session status + inserts results).
	// Circuit breaker still protects against cascading failures.
	_, err := s.cb.Execute(func() (any, error) {
		tx, txErr := s.pool.Begin(ctx)
		if txErr != nil {
			return nil, fmt.Errorf("begin tx: %w", txErr)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		// 1. Update game_sessions status to 'ended'
		if _, execErr := tx.Exec(ctx,
			`UPDATE game_sessions SET status = 'ended', ended_at = $1, final_score = $2 WHERE id = $3`,
			endedAt, finalScore, sessionID); execErr != nil {
			return nil, fmt.Errorf("end game session: %w", execErr)
		}

		// 2. Insert all game results in a single multi-value INSERT (N+1 fix).
		// 企业为何需要：循环内逐条 INSERT 产生 N 次网络往返，多值 INSERT 将 N 条合并为 1 次查询。
		// ON CONFLICT (id) DO NOTHING ensures idempotency for async worker retries.
		if len(results) > 0 {
			var placeholders []string
			var values []interface{}
			for i, r := range results {
				base := i * 6 // 6 params per row
				placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d)", base+1, base+2, base+3, base+4, base+5, base+6))
				values = append(values, r.ID, r.SessionID, r.UserID, r.ScoreContribution, r.TapsCount, r.CreatedAt)
			}
			query := fmt.Sprintf("INSERT INTO game_results (id, session_id, user_id, score_contribution, taps_count, created_at) VALUES %s ON CONFLICT (id) DO NOTHING", strings.Join(placeholders, ","))
			if _, execErr := tx.Exec(ctx, query, values...); execErr != nil {
				return nil, fmt.Errorf("insert game results: %w", execErr)
			}
		}

		// 3. Commit — both session end and results are atomic
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return nil, fmt.Errorf("commit end game and results: %w", commitErr)
		}
		return nil, nil
	})
	return err
}

// GetGameResultsByUserID returns the most recent game results for a user.
// Uses the idx_game_results_user_id index for efficient lookup.
// 企业为何需要：ExportUserData 原返回空数组，用户无法导出实际游戏数据（GDPR 第 20 条数据可携带权）。
// N+1 fix: single indexed query replaces per-session lookups.
func (s *PostgresStore) GetGameResultsByUserID(ctx context.Context, userID string) ([]domain.GameResult, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.GetGameResultsByUserID",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "SELECT"),
			attribute.String("db.statement", "SELECT id, session_id, user_id, score_contribution, taps_count, created_at FROM game_results WHERE user_id = $1 ORDER BY created_at DESC LIMIT 100"),
		),
	)
	defer span.End()

	var results []domain.GameResult
	err := s.withRetryRead(ctx, func(ctx context.Context) error {
		rows, err := s.pool.Query(ctx,
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

// LobbyListResult contains paginated lobby results with metadata.
// 企业为何需要：Offset 分页在深页（大 offset）时性能差，需扫描并丢弃前 N 行。
// Cursor 分页利用索引直接定位，性能恒定。这是"offset 分页有什么问题"的标准面试答案。
type LobbyListResult struct {
	Lobbies    []domain.LobbyState
	Total      int
	HasMore    bool
	NextCursor string // format: "updated_at|code"
}

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

// LoadAllActiveLobbies returns lobby states with cursor-based pagination.
// If limit <= 0, defaults to 50. If limit > 100, caps at 100.
// Cursor format: "updated_at|code". When cursor is provided, uses keyset pagination
// via WHERE (updated_at, code) < (cursor_updated_at, cursor_code).
// Always fetches limit+1 rows to determine has_more.
// Returns total count via a separate COUNT query.
// 企业为何需要：Offset 分页在深页（大 offset）时性能差，需扫描并丢弃前 N 行。
// Cursor 分页利用索引直接定位，性能恒定。这是"offset 分页有什么问题"的标准面试答案。
func (s *PostgresStore) LoadAllActiveLobbies(ctx context.Context, limit int, cursor string) (*LobbyListResult, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	spanAttrs := []attribute.KeyValue{
		attribute.String("db.system", "postgresql"),
		attribute.Int("db.limit", limit),
	}
	if cursor != "" {
		spanAttrs = append(spanAttrs, attribute.String("db.cursor", cursor))
	}

	ctx, span := telemetry.Tracer().Start(ctx, "postgres.LoadAllActiveLobbies",
		trace.WithAttributes(spanAttrs...),
	)
	defer span.End()

	cursorUpdatedAt, cursorCode := parseLobbyCursor(cursor)

	total, err := s.countAllLobbies(ctx)
	if err != nil {
		return nil, err
	}

	lobbies, err := s.fetchLobbiesPage(ctx, limit+1, cursorUpdatedAt, cursorCode)
	if err != nil {
		return nil, err
	}

	return buildLobbyListResult(lobbies, total, limit), nil
}

// parseLobbyCursor parses the "updated_at|code" cursor format.
func parseLobbyCursor(cursor string) (cursorUpdatedAt int64, cursorCode string) {
	if cursor == "" {
		return 0, ""
	}
	parts := strings.SplitN(cursor, "|", 2)
	if len(parts) == 2 {
		if v, err := fmt.Sscanf(parts[0], "%d", &cursorUpdatedAt); err != nil || v != 1 {
			cursorUpdatedAt = 0
		}
		cursorCode = parts[1]
	}
	return cursorUpdatedAt, cursorCode
}

// countAllLobbies returns the total number of lobby_states rows.
func (s *PostgresStore) countAllLobbies(ctx context.Context) (int, error) {
	var total int
	err := s.withRetryRead(ctx, func(ctx context.Context) error {
		if countErr := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM lobby_states`).Scan(&total); countErr != nil {
			return fmt.Errorf("count lobbies: %w", countErr)
		}
		return nil
	})
	return total, err
}

// fetchLobbiesPage fetches a page of lobby states using cursor-based pagination.
// When cursorUpdatedAt > 0, uses keyset pagination; otherwise fetches the first page.
func (s *PostgresStore) fetchLobbiesPage(ctx context.Context, fetchLimit int, cursorUpdatedAt int64, cursorCode string) ([]domain.LobbyState, error) {
	var lobbies []domain.LobbyState
	err := s.withRetryRead(ctx, func(ctx context.Context) error {
		rows, queryErr := s.queryLobbies(ctx, fetchLimit, cursorUpdatedAt, cursorCode)
		if queryErr != nil {
			return fmt.Errorf("load all lobbies: %w", queryErr)
		}
		defer rows.Close()

		result, err := scanLobbyRows(rows)
		if err != nil {
			return err
		}
		lobbies = result
		return nil
	})
	return lobbies, err
}

// queryLobbies builds and executes the lobby query based on whether a cursor is provided.
func (s *PostgresStore) queryLobbies(ctx context.Context, fetchLimit int, cursorUpdatedAt int64, cursorCode string) (pgx.Rows, error) {
	if cursorUpdatedAt > 0 {
		return s.pool.Query(ctx,
			`SELECT id, code, state, updated_at, created_at FROM lobby_states
			 WHERE (updated_at, code) < ($1, $2)
			 ORDER BY updated_at DESC, code DESC LIMIT $3`,
			cursorUpdatedAt, cursorCode, fetchLimit)
	}
	return s.pool.Query(ctx,
		`SELECT id, code, state, updated_at, created_at FROM lobby_states
		 ORDER BY updated_at DESC, code DESC LIMIT $1`,
		fetchLimit)
}

// scanLobbyRows scans all rows into a slice of LobbyState.
func scanLobbyRows(rows pgx.Rows) ([]domain.LobbyState, error) {
	var result []domain.LobbyState
	for rows.Next() {
		var ls domain.LobbyState
		if scanErr := rows.Scan(&ls.ID, &ls.Code, &ls.State, &ls.UpdatedAt, &ls.CreatedAt); scanErr != nil {
			return nil, fmt.Errorf("scan lobby: %w", scanErr)
		}
		result = append(result, ls)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("iterate lobbies: %w", rowsErr)
	}
	return result, nil
}

// buildLobbyListResult trims to limit, determines has_more, and builds the next cursor.
func buildLobbyListResult(lobbies []domain.LobbyState, total int, limit int) *LobbyListResult {
	hasMore := len(lobbies) > limit
	if hasMore {
		lobbies = lobbies[:limit]
	}

	var nextCursor string
	if hasMore && len(lobbies) > 0 {
		last := lobbies[len(lobbies)-1]
		nextCursor = fmt.Sprintf("%d|%s", last.UpdatedAt, last.Code)
	}

	return &LobbyListResult{
		Lobbies:    lobbies,
		Total:      total,
		HasMore:    hasMore,
		NextCursor: nextCursor,
	}
}

// DeleteLobbyState removes a lobby state by code.
// No retry: non-idempotent (deletion side effects).
func (s *PostgresStore) DeleteLobbyState(ctx context.Context, code string) error {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.DeleteLobbyState",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "DELETE"),
			attribute.String("db.statement", "DELETE FROM lobby_states WHERE code = $1"),
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

// --- Admin Config ---

// GetConfig loads an admin config by ID. Returns nil if not found.
func (s *PostgresStore) GetConfig(ctx context.Context, id string) (*domain.AppConfig, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.GetConfig",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "SELECT"),
			attribute.String("db.statement", "SELECT id, config, updated_at FROM admin_config WHERE id = $1"),
		),
	)
	defer span.End()

	var c *domain.AppConfig
	err := s.withRetryRead(ctx, func(ctx context.Context) error {
		row := s.pool.QueryRow(ctx,
			`SELECT id, config, updated_at FROM admin_config WHERE id = $1`, id)

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
// Retry OK: UPSERT is idempotent.
func (s *PostgresStore) SaveConfig(ctx context.Context, c *domain.AppConfig) error {
	ctx, span := telemetry.Tracer().Start(ctx, "postgres.SaveConfig",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", "UPSERT"),
			attribute.String("db.statement", "INSERT INTO admin_config (id, config, updated_at) VALUES ($1, $2, $3) ON CONFLICT (id) DO UPDATE SET config = EXCLUDED.config, updated_at = EXCLUDED.updated_at"),
		),
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
