// Package store provides PostgreSQL and Redis data access layers.
// store-022: This file provides the PostgreSQL connection pool wrapper
// (PostgresStore), pool configuration, migration runner, and repository
// factory functions. Package-level documentation lives in doc.go.
package store

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sethvargo/go-retry"
	"github.com/sony/gobreaker/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/migrateutil"
)

var ErrDuplicateUser = domain.ErrDuplicateUser

// pgPool is an alias for PGPool kept for backward compatibility with
// existing repository constructors.
type pgPool = PGPool

type baseRepository struct {
	pool pgPool
	cb   *gobreaker.CircuitBreaker[any]
	deps Deps
}

func newBaseRepository(pool pgPool, deps Deps) baseRepository {
	return baseRepository{pool: pool, cb: deps.PostgresBreakerFactory(), deps: deps}
}

// withRetry wraps fn with the DB retry policy and circuit breaker.
// Read/write distinction is handled by MaybeRetryableFn + breaker MaxRequests,
// so both paths share the same implementation.
func (b *baseRepository) withRetry(ctx context.Context, fn func(context.Context) error) error {
	return retry.Do(ctx, b.deps.DBRetryPolicy, func(ctx context.Context) error {
		_, err := b.cb.Execute(func() (any, error) {
			return nil, fn(ctx)
		})
		return b.deps.MaybeRetryableFn(err)
	})
}

// withSpan starts a named tracing span, executes fn, and ends the span.
// Attributes are optional key-value pairs appended to the span.
func withSpan(ctx context.Context, tracer trace.Tracer, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	ctx, span := tracer.Start(ctx, name,
		trace.WithAttributes(append([]attribute.KeyValue{
			attribute.String("db.system", "postgresql"),
		}, attrs...)...),
	)
	return ctx, span
}

// PostgresStore provides PostgreSQL-backed persistence.
type PostgresStore struct {
	pool PGPool
	cb   *gobreaker.CircuitBreaker[any]
	deps Deps

	lobby  *LobbyRepository
	result *ResultRepository

	lastAcquireDuration atomic.Value // float64
	lastAcquireCount    atomic.Value // int64
}

// pgxNewWithConfigFn is replaceable in unit tests to avoid a live PostgreSQL instance.
var pgxNewWithConfigFn = func(ctx context.Context, cfg *pgxpool.Config) (PGPool, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil pool config")
	}
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// NewPostgresStore creates a connection pool and validates connectivity.
func NewPostgresStore(connString string, timeouts config.TimeoutConfig, deps ...Deps) (*PostgresStore, error) {
	d := depsOrZero(deps...)
	ctx, cancel := context.WithTimeout(context.Background(), timeouts.PGConnectTimeout)
	defer cancel()

	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	poolConfig.MaxConns = int32(config.GetEnvIntPositive("PG_POOL_MAX_CONNS", 25)) //nolint:gosec // G115: bounded by config validation (positive int)
	poolConfig.MinConns = int32(config.GetEnvIntPositive("PG_POOL_MIN_CONNS", 5))  //nolint:gosec // G115: bounded by config validation (positive int)
	poolConfig.MaxConnLifetime = config.GetEnvDuration("PG_POOL_MAX_CONN_LIFETIME", 30*time.Minute)
	poolConfig.MaxConnIdleTime = config.GetEnvDuration("PG_POOL_MAX_CONN_IDLE_TIME", 5*time.Minute)
	poolConfig.HealthCheckPeriod = config.GetEnvDuration("PG_POOL_HEALTH_CHECK_PERIOD", 30*time.Second)

	poolConfig.PrepareConn = func(_ context.Context, _ *pgx.Conn) (bool, error) {
		d.PoolMetrics.IncAcquireCount()
		return true, nil
	}

	pool, err := pgxNewWithConfigFn(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	return &PostgresStore{
		pool:   pool,
		cb:     d.PostgresBreakerFactory(),
		deps:   d,
		lobby:  NewLobbyRepository(pool, d),
		result: NewResultRepository(pool, d),
	}, nil
}

// NewPostgresStoreWithPool wraps an existing pool (pgxmock-backed unit tests).
func NewPostgresStoreWithPool(pool PGPool, deps ...Deps) *PostgresStore {
	d := depsOrZero(deps...)
	return &PostgresStore{
		pool:   pool,
		cb:     d.PostgresBreakerFactory(),
		deps:   d,
		lobby:  NewLobbyRepository(pool, d),
		result: NewResultRepository(pool, d),
	}
}

// CircuitBreaker returns the PostgreSQL circuit breaker for degradation detection.
func (s *PostgresStore) CircuitBreaker() *gobreaker.CircuitBreaker[any] { return s.cb }

// Close releases the connection pool.
func (s *PostgresStore) Close() {
	s.pool.Close()
}

// SaveLobbyState persists lobby state via the LobbyRepository.
func (s *PostgresStore) SaveLobbyState(ctx context.Context, ls *domain.LobbyState) error {
	return s.lobby.SaveLobbyState(ctx, ls)
}

// LoadLobbyState retrieves lobby state by code via the LobbyRepository.
func (s *PostgresStore) LoadLobbyState(ctx context.Context, code string) (*domain.LobbyState, error) {
	return s.lobby.LoadLobbyState(ctx, code)
}

// DeleteLobbyState removes lobby state by code via the LobbyRepository.
func (s *PostgresStore) DeleteLobbyState(ctx context.Context, code string) error {
	return s.lobby.DeleteLobbyState(ctx, code)
}

// LoadAllActiveLobbies returns paginated active lobbies via the LobbyRepository.
func (s *PostgresStore) LoadAllActiveLobbies(ctx context.Context, limit int, cursor string) (*domain.LobbyListResult, error) {
	return s.lobby.LoadAllActiveLobbies(ctx, limit, cursor)
}

// CreateGameSession inserts a new game session via the ResultRepository.
func (s *PostgresStore) CreateGameSession(ctx context.Context, gs *domain.GameSession) error {
	return s.result.CreateGameSession(ctx, gs)
}

// RecordGameResult records final game results via the ResultRepository.
func (s *PostgresStore) RecordGameResult(ctx context.Context, sessionID, roomCode string, endedAt int64, finalScore int, results []domain.GameResultPlayer) error {
	return s.result.RecordGameResult(ctx, sessionID, roomCode, endedAt, finalScore, results)
}

// Pool returns the underlying connection pool.
func (s *PostgresStore) Pool() *pgxpool.Pool {
	p, _ := s.pool.(*pgxpool.Pool)
	return p
}

// NewUserRepository returns a UserRepository backed by this store's pool.
// Unlike NewUserRepository(db.Pool()), this works with pgxmock-backed stores in tests.
func (s *PostgresStore) NewUserRepository() *UserRepository {
	return NewUserRepository(s.pool, s.deps)
}

// PoolStats returns the current connection pool statistics.
func (s *PostgresStore) PoolStats() *pgxpool.Stat {
	if p, ok := s.pool.(*pgxpool.Pool); ok {
		return p.Stat()
	}
	return nil
}

// runMigrationsFn is replaceable in unit tests to avoid a live PostgreSQL instance.
var runMigrationsFn = migrateutil.RunMigrations

// SetRunMigrationsHook replaces RunMigrations behavior in unit tests; returns restore.
func SetRunMigrationsHook(fn func(context.Context, string, string) error) func() {
	orig := runMigrationsFn
	if fn != nil {
		runMigrationsFn = fn
	}
	return func() { runMigrationsFn = orig }
}

// RunMigrations applies all pending migrations from the given directory.
func (s *PostgresStore) RunMigrations(migrationsPath string) error {
	p, ok := s.pool.(*pgxpool.Pool)
	if !ok {
		return fmt.Errorf("migrations require a real pgxpool connection")
	}
	ctx := context.Background()
	if err := runMigrationsFn(ctx, p.Config().ConnString(), migrationsPath); err != nil {
		return err
	}
	return nil
}

// ObservePoolStats publishes pgx pool saturation metrics to Prometheus.
func (s *PostgresStore) ObservePoolStats() {
	p, ok := s.pool.(*pgxpool.Pool)
	if !ok {
		return
	}
	stat := p.Stat()
	s.deps.PoolMetrics.SetIdleConns(float64(stat.IdleConns()))
	s.deps.PoolMetrics.SetInUseConns(float64(stat.AcquiredConns()))
	s.recordAcquireDurationDelta(stat.AcquireDuration().Seconds(), stat.AcquireCount())
}

func (s *PostgresStore) recordAcquireDurationDelta(currentDuration float64, currentCount int64) {
	if prevDur, ok := s.lastAcquireDuration.Load().(float64); ok && prevDur > 0 {
		if prevCnt, ok := s.lastAcquireCount.Load().(int64); ok && currentCount > prevCnt {
			delta := currentDuration - prevDur
			countDelta := float64(currentCount - prevCnt)
			if delta > 0 && countDelta > 0 {
				s.deps.PoolMetrics.ObserveAcquireDuration(delta / countDelta)
			}
		}
	}
	s.lastAcquireDuration.Store(currentDuration)
	s.lastAcquireCount.Store(currentCount)
}
