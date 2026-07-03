// Package store provides PostgreSQL and Redis storage wrappers for the game backend.
package store

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sony/gobreaker/v2"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/migrateutil"
	"github.com/uppy-clone/backend/internal/resilience"
)

// pgPool abstracts pgxpool for store operations (enables pgxmock in unit tests).
type pgPool interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Close()
	Ping(ctx context.Context) error
}

// ErrDuplicateUser indicates a unique constraint violation on user creation.
var ErrDuplicateUser = domain.ErrDuplicateUser

// PostgresStore provides PostgreSQL-backed persistence.
type PostgresStore struct {
	pool pgPool
	cb   *gobreaker.CircuitBreaker[any]

	lastAcquireDuration atomic.Value // float64
	lastAcquireCount    atomic.Value // int64
}

// pgxNewWithConfigFn is replaceable in unit tests to avoid a live PostgreSQL instance.
var pgxNewWithConfigFn = func(ctx context.Context, cfg *pgxpool.Config) (pgPool, error) {
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
func NewPostgresStore(connString string, timeouts config.TimeoutConfig) (*PostgresStore, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeouts.PGConnectTimeout)
	defer cancel()

	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	poolConfig.MaxConns = int32(config.GetEnvIntPositive("PG_POOL_MAX_CONNS", 25)) //nolint:gosec:G115
	poolConfig.MinConns = int32(config.GetEnvIntPositive("PG_POOL_MIN_CONNS", 5))  //nolint:gosec:G115
	poolConfig.MaxConnLifetime = config.GetEnvDuration("PG_POOL_MAX_CONN_LIFETIME", 30*time.Minute)
	poolConfig.MaxConnIdleTime = config.GetEnvDuration("PG_POOL_MAX_CONN_IDLE_TIME", 5*time.Minute)
	poolConfig.HealthCheckPeriod = config.GetEnvDuration("PG_POOL_HEALTH_CHECK_PERIOD", 30*time.Second)

	poolConfig.PrepareConn = func(_ context.Context, _ *pgx.Conn) (bool, error) {
		metrics.DBPoolAcquireCount.Inc()
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
		pool: pool,
		cb:   resilience.NewPostgresBreaker(),
	}, nil
}

// NewPostgresStoreWithPool wraps an existing pool (pgxmock-backed unit tests).
func NewPostgresStoreWithPool(pool pgPool) *PostgresStore {
	return &PostgresStore{
		pool: pool,
		cb:   resilience.NewPostgresBreaker(),
	}
}

// Close releases the connection pool.
func (s *PostgresStore) Close() {
	s.pool.Close()
}

// Pool returns the underlying connection pool.
func (s *PostgresStore) Pool() *pgxpool.Pool {
	p, _ := s.pool.(*pgxpool.Pool)
	return p
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
