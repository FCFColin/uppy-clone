package config

import (
	"time"
)

// TimeoutConfig holds timeout durations for various operations.
//
// Enterprise rationale: Hardcoded timeouts cannot be tuned for different
// environments (dev vs staging vs prod). Connection, read, and request
// timeouts serve different purposes and must be independently configurable:
// - Connect timeout: TCP handshake time (network RTT)
// - Read timeout: waiting for first byte of response (server processing)
// - Request timeout: total time including retries (user-facing SLA)
// Trade-off: More config = more complexity, but the alternative is
// redeploying to change a timeout value.
type TimeoutConfig struct {
	// PostgreSQL
	PGConnectTimeout time.Duration
	PGQueryTimeout   time.Duration
	PGRequestTimeout time.Duration

	// Redis
	RedisConnectTimeout time.Duration
	RedisReadTimeout    time.Duration
	RedisWriteTimeout   time.Duration

	// HTTP (external APIs)
	HTTPConnectTimeout time.Duration
	HTTPRequestTimeout time.Duration

	// WebSocket
	WSWriteTimeout time.Duration
	WSPongTimeout  time.Duration
	WSPingInterval time.Duration
}

// DefaultTimeoutConfig returns production-ready defaults.
func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		PGConnectTimeout: GetEnvDuration("PG_CONNECT_TIMEOUT", 5*time.Second),
		PGQueryTimeout:   GetEnvDuration("PG_QUERY_TIMEOUT", 10*time.Second),
		PGRequestTimeout: GetEnvDuration("PG_REQUEST_TIMEOUT", 30*time.Second),

		RedisConnectTimeout: GetEnvDuration("REDIS_CONNECT_TIMEOUT", 3*time.Second),
		RedisReadTimeout:    GetEnvDuration("REDIS_READ_TIMEOUT", 3*time.Second),
		RedisWriteTimeout:   GetEnvDuration("REDIS_WRITE_TIMEOUT", 3*time.Second),

		HTTPConnectTimeout: GetEnvDuration("HTTP_CONNECT_TIMEOUT", 5*time.Second),
		HTTPRequestTimeout: GetEnvDuration("HTTP_REQUEST_TIMEOUT", 10*time.Second),

		WSWriteTimeout: GetEnvDuration("WS_WRITE_TIMEOUT", 10*time.Second),
		WSPongTimeout:  GetEnvDuration("WS_PONG_TIMEOUT", 60*time.Second),
		WSPingInterval: GetEnvDuration("WS_PING_INTERVAL", 30*time.Second),
	}
}
