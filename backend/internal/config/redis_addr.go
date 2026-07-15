package config

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// RedisConn holds parsed REDIS_URL components for go-redis.
type RedisConn struct {
	Addr     string
	Password string
	DB       int // misc-022: Redis database number from URL path (e.g., redis://host:6379/2)
}

// ParseRedisURL accepts host:port or redis://[:password@]host:port[/db].
func ParseRedisURL(raw string) (RedisConn, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return RedisConn{}, fmt.Errorf("REDIS_URL is empty")
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
	// misc-022: Parse the DB number from the URL path (e.g., "/2" → DB=2).
	// Previously silently ignored, causing unexpected default-DB usage.
	if u.Path != "" && u.Path != "/" {
		dbStr := strings.TrimPrefix(u.Path, "/")
		db, err := strconv.Atoi(dbStr)
		if err != nil || db < 0 {
			return RedisConn{}, fmt.Errorf("parse REDIS_URL: invalid DB number %q", dbStr)
		}
		conn.DB = db
	}
	return conn, nil
}
