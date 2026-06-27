// Package store provides PostgreSQL and Redis storage wrappers for the game backend.
package store

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sony/gobreaker/v2"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/migrateutil"
	"github.com/uppy-clone/backend/internal/resilience"
)

// ErrDuplicateUser indicates a unique constraint violation on user creation.
var ErrDuplicateUser = errors.New("duplicate user")

// PostgresStore provides PostgreSQL-backed persistence.
type PostgresStore struct {
	pool *pgxpool.Pool
	cb   *gobreaker.CircuitBreaker[any]

	lastAcquireDuration atomic.Value // float64
	lastAcquireCount    atomic.Value // int64
}

// NewPostgresStore creates a connection pool and validates connectivity.
func NewPostgresStore(connString string, timeouts config.TimeoutConfig) (*PostgresStore, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeouts.PGConnectTimeout)
	defer cancel()

	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	poolConfig.MaxConns = int32(getEnvInt("PG_POOL_MAX_CONNS", 25)) //nolint:gosec
	poolConfig.MinConns = int32(getEnvInt("PG_POOL_MIN_CONNS", 5))  //nolint:gosec
	poolConfig.MaxConnLifetime = getEnvDuration("PG_POOL_MAX_CONN_LIFETIME", 30*time.Minute)
	poolConfig.MaxConnIdleTime = getEnvDuration("PG_POOL_MAX_CONN_IDLE_TIME", 5*time.Minute)
	poolConfig.HealthCheckPeriod = getEnvDuration("PG_POOL_HEALTH_CHECK_PERIOD", 30*time.Second)

	poolConfig.PrepareConn = func(_ context.Context, _ *pgx.Conn) (bool, error) {
		metrics.DBPoolAcquireCount.Inc()
		return true, nil
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
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

// Pool returns the underlying connection pool.
func (s *PostgresStore) Pool() *pgxpool.Pool {
	return s.pool
}

// PoolStats returns the current connection pool statistics.
func (s *PostgresStore) PoolStats() *pgxpool.Stat {
	return s.pool.Stat()
}

// RunMigrations applies all pending migrations from the given directory.
func (s *PostgresStore) RunMigrations(migrationsPath string) error {
	ctx := context.Background()
	if err := migrateutil.RunMigrations(ctx, s.pool.Config().ConnString(), migrationsPath); err != nil {
		return err
	}
	return nil
}
