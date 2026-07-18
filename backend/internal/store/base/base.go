// Package base provides shared infrastructure types for store sub-packages.
// It contains the pool/metrics abstractions used by store sub-packages.
package base

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ─── Pool & Metrics ──────────────────────────────────────────────────

// PGPool abstracts pgxpool for store operations (enables pgxmock in unit tests).
type PGPool interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Close()
	Ping(ctx context.Context) error
}

// PoolMetricsRecorder abstracts Prometheus metrics for the PG connection pool.
type PoolMetricsRecorder interface {
	IncAcquireCount()
	SetIdleConns(val float64)
	SetInUseConns(val float64)
	ObserveAcquireDuration(val float64)
}
