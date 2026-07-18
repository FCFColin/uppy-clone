// Package health provides liveness and readiness HTTP handlers.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker/v2"
)

const healthCheckTimeout = 500 * time.Millisecond
const unavailableStatus = "unavailable"

// Checker runs PostgreSQL, Redis, WebSocket, and circuit-breaker health checks.
type Checker struct {
	pool            *pgxpool.Pool
	redis           *redis.Client
	canAcceptWS     func() bool
	circuitBreakers []*gobreaker.CircuitBreaker[any]
	poolPing        func(context.Context) error
}

// NewChecker creates a Checker for the given PostgreSQL pool and Redis client.
func NewChecker(pool *pgxpool.Pool, rdb *redis.Client) *Checker {
	return &Checker{pool: pool, redis: rdb}
}

// WithCanAcceptWS registers a callback that reports whether the server can accept WebSocket connections.
func (c *Checker) WithCanAcceptWS(fn func() bool) *Checker {
	c.canAcceptWS = fn
	return c
}

// WithPoolPing overrides the PostgreSQL ping function (for unit tests).
func (c *Checker) WithPoolPing(fn func(context.Context) error) *Checker {
	c.poolPing = fn
	return c
}

// WithCircuitBreakers registers circuit breakers whose state contributes to readiness.
func (c *Checker) WithCircuitBreakers(cbs ...*gobreaker.CircuitBreaker[any]) *Checker {
	c.circuitBreakers = cbs
	return c
}

// LiveHandler reports liveness with a 200 response.
func (c *Checker) LiveHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "alive"})
}

// ReadyHandler reports readiness based on PostgreSQL, Redis, WebSocket, and circuit-breaker state.
func (c *Checker) ReadyHandler(w http.ResponseWriter, r *http.Request) { //nolint:funlen // HTTP handler with multiple health checks
	checks := map[string]string{}
	pgOK := true
	redisOK := true

	if c.pool != nil {
		ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
		defer cancel()
		var err error
		if c.poolPing != nil {
			err = c.poolPing(ctx)
		} else {
			err = c.pool.Ping(ctx)
		}
		if err != nil {
			checks["postgres"] = unavailableStatus
			pgOK = false
		} else {
			checks["postgres"] = "ok"
		}
	}

	if c.redis != nil {
		ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
		defer cancel()
		if err := c.redis.Ping(ctx).Err(); err != nil {
			checks["redis"] = unavailableStatus
			redisOK = false
		} else {
			checks["redis"] = "ok"
		}
	}

	wsOK := true
	checks["websocket"] = "ok"
	if c.canAcceptWS != nil && !c.canAcceptWS() {
		checks["websocket"] = "at capacity"
		wsOK = false
	}

	cbOK := true
	for _, cb := range c.circuitBreakers {
		if cb != nil {
			state := cb.State()
			if state == gobreaker.StateOpen || state == gobreaker.StateHalfOpen {
				cbOK = false
				break
			}
		}
	}
	if !cbOK {
		checks["circuit_breaker"] = "open"
	}

	status := "ready"
	code := http.StatusOK
	if !pgOK || !wsOK || !cbOK {
		status = "not ready"
		code = http.StatusServiceUnavailable
	} else if !redisOK {
		// handler-026: Return 503 when Redis is down so load balancers stop
		// routing traffic to this instance. Previously returned 200 "degraded",
		// which allowed traffic to continue hitting an instance with no
		// rate limiting or session management.
		status = "degraded"
		code = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": status,
		"checks": checks,
	})
}
