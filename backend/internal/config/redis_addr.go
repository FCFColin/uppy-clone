package config

import (
	"fmt"
	"net/url"
	"strings"
)

// RedisConn holds parsed REDIS_URL components for go-redis.
type RedisConn struct {
	Addr     string
	Password string
}

// ParseRedisURL accepts host:port or redis://[:password@]host:port[/db].
func ParseRedisURL(raw string) (RedisConn, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return RedisConn{Addr: "localhost:6379"}, nil
	}
	if !strings.HasPrefix(raw, "redis://") && !strings.HasPrefix(raw, "rediss://") {
		return RedisConn{Addr: raw}, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return RedisConn{}, fmt.Errorf("parse REDIS_URL: %w", err)
	}
	if u.Host == "" {
		return RedisConn{}, fmt.Errorf("parse REDIS_URL: missing host")
	}
	conn := RedisConn{Addr: u.Host}
	if u.User != nil {
		conn.Password, _ = u.User.Password()
	}
	return conn, nil
}
