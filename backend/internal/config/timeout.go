package config

import (
	"os"
	"strconv"
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
		PGConnectTimeout:  getDurationEnv("PG_CONNECT_TIMEOUT", 5*time.Second),
		PGQueryTimeout:    getDurationEnv("PG_QUERY_TIMEOUT", 10*time.Second),
		PGRequestTimeout:  getDurationEnv("PG_REQUEST_TIMEOUT", 30*time.Second),

		RedisConnectTimeout: getDurationEnv("REDIS_CONNECT_TIMEOUT", 3*time.Second),
		RedisReadTimeout:    getDurationEnv("REDIS_READ_TIMEOUT", 3*time.Second),
		RedisWriteTimeout:   getDurationEnv("REDIS_WRITE_TIMEOUT", 3*time.Second),

		HTTPConnectTimeout: getDurationEnv("HTTP_CONNECT_TIMEOUT", 5*time.Second),
		HTTPRequestTimeout: getDurationEnv("HTTP_REQUEST_TIMEOUT", 10*time.Second),

		WSWriteTimeout: getDurationEnv("WS_WRITE_TIMEOUT", 10*time.Second),
		WSPongTimeout:  getDurationEnv("WS_PONG_TIMEOUT", 60*time.Second),
		WSPingInterval: getDurationEnv("WS_PING_INTERVAL", 30*time.Second),
	}
}

func getDurationEnv(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if seconds, err := strconv.Atoi(val); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}
	return defaultVal
}
