package store

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/uppy-clone/backend/internal/config"
)

func TestNewRedisStore_Miniredis(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	timeouts := config.TimeoutConfig{
		RedisConnectTimeout: time.Second,
		RedisReadTimeout:    time.Second,
		RedisWriteTimeout:   time.Second,
	}
	s, err := NewRedisStore(mr.Addr(), timeouts)
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if s.Client() == nil {
		t.Fatal("Client() returned nil")
	}
	stats := s.PoolStats()
	if stats == nil {
		t.Fatal("PoolStats() returned nil")
	}
}

func TestNewRedisStore_InvalidURL(t *testing.T) {
	_, err := NewRedisStore("redis://", config.TimeoutConfig{
		RedisConnectTimeout: 100 * time.Millisecond,
		RedisReadTimeout:    100 * time.Millisecond,
		RedisWriteTimeout:   100 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected error for invalid redis URL")
	}
}

func TestNewRedisStore_UnreachablePing(t *testing.T) {
	_, err := NewRedisStore("127.0.0.1:1", config.TimeoutConfig{
		RedisConnectTimeout: 100 * time.Millisecond,
		RedisReadTimeout:    100 * time.Millisecond,
		RedisWriteTimeout:   100 * time.Millisecond,
	})
	if err == nil || !strings.Contains(err.Error(), "redis ping") {
		t.Fatalf("NewRedisStore = %v, want redis ping error", err)
	}
}

func TestNewRedisStore_PoolEnvOverrides(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	os.Setenv("REDIS_POOL_SIZE", "8")
	os.Setenv("REDIS_MIN_IDLE_CONNS", "2")
	t.Cleanup(func() {
		os.Unsetenv("REDIS_POOL_SIZE")
		os.Unsetenv("REDIS_MIN_IDLE_CONNS")
	})

	s, err := NewRedisStore(mr.Addr(), config.DefaultTimeoutConfig())
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if s.Client().Options().PoolSize != 8 {
		t.Errorf("PoolSize = %d, want 8", s.Client().Options().PoolSize)
	}
	if s.Client().Options().MinIdleConns != 2 {
		t.Errorf("MinIdleConns = %d, want 2", s.Client().Options().MinIdleConns)
	}
}

func TestRedisStore_Close(t *testing.T) {
	s, _ := newTestRedisStore(t)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
