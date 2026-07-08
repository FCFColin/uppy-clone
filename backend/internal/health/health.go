package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const healthCheckTimeout = 500 * time.Millisecond
const unavailableStatus = "unavailable"

var poolPingForTest func(context.Context) error

type Checker struct {
	pool        *pgxpool.Pool
	redis       *redis.Client
	canAcceptWS func() bool
}

func NewChecker(pool *pgxpool.Pool, rdb *redis.Client) *Checker {
	return &Checker{pool: pool, redis: rdb}
}

func (c *Checker) WithCanAcceptWS(fn func() bool) *Checker {
	c.canAcceptWS = fn
	return c
}

func (c *Checker) LiveHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "alive"})
}

func (c *Checker) ReadyHandler(w http.ResponseWriter, r *http.Request) {
	checks := map[string]string{}
	pgOK := true
	redisOK := true

	if c.pool != nil {
		ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
		defer cancel()
		var err error
		if poolPingForTest != nil {
			err = poolPingForTest(ctx)
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

	status := "ready"
	code := http.StatusOK
	if !pgOK || !wsOK {
		status = "not ready"
		code = http.StatusServiceUnavailable
	} else if !redisOK {
		status = "degraded"
		code = http.StatusOK
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": status,
		"checks": checks,
	})
}
