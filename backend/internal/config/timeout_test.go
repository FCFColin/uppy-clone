package config

import (
	"os"
	"testing"
	"time"
)

// ─── DefaultTimeoutConfig ────────────────────────────────────────────

func TestDefaultTimeoutConfig(t *testing.T) {
	cfg := DefaultTimeoutConfig()

	// PostgreSQL
	if cfg.PGConnectTimeout != 5*time.Second {
		t.Fatalf("PGConnectTimeout: got %v, want 5s", cfg.PGConnectTimeout)
	}
	if cfg.PGQueryTimeout != 10*time.Second {
		t.Fatalf("PGQueryTimeout: got %v, want 10s", cfg.PGQueryTimeout)
	}
	if cfg.PGRequestTimeout != 30*time.Second {
		t.Fatalf("PGRequestTimeout: got %v, want 30s", cfg.PGRequestTimeout)
	}

	// Redis
	if cfg.RedisConnectTimeout != 3*time.Second {
		t.Fatalf("RedisConnectTimeout: got %v, want 3s", cfg.RedisConnectTimeout)
	}
	if cfg.RedisReadTimeout != 3*time.Second {
		t.Fatalf("RedisReadTimeout: got %v, want 3s", cfg.RedisReadTimeout)
	}
	if cfg.RedisWriteTimeout != 3*time.Second {
		t.Fatalf("RedisWriteTimeout: got %v, want 3s", cfg.RedisWriteTimeout)
	}

	// HTTP
	if cfg.HTTPConnectTimeout != 5*time.Second {
		t.Fatalf("HTTPConnectTimeout: got %v, want 5s", cfg.HTTPConnectTimeout)
	}
	if cfg.HTTPRequestTimeout != 10*time.Second {
		t.Fatalf("HTTPRequestTimeout: got %v, want 10s", cfg.HTTPRequestTimeout)
	}

	// WebSocket
	if cfg.WSWriteTimeout != 10*time.Second {
		t.Fatalf("WSWriteTimeout: got %v, want 10s", cfg.WSWriteTimeout)
	}
	if cfg.WSPongTimeout != 60*time.Second {
		t.Fatalf("WSPongTimeout: got %v, want 60s", cfg.WSPongTimeout)
	}
	if cfg.WSPingInterval != 30*time.Second {
		t.Fatalf("WSPingInterval: got %v, want 30s", cfg.WSPingInterval)
	}
}

// ─── Environment variable overrides ──────────────────────────────────

func TestTimeoutConfigFromEnv_PGConnect(t *testing.T) {
	_ = os.Setenv("PG_CONNECT_TIMEOUT", "10")
	defer func() { _ = os.Unsetenv("PG_CONNECT_TIMEOUT") }()

	cfg := DefaultTimeoutConfig()
	if cfg.PGConnectTimeout != 10*time.Second {
		t.Fatalf("PGConnectTimeout from env: got %v, want 10s", cfg.PGConnectTimeout)
	}
}

func TestTimeoutConfigFromEnv_PGQuery(t *testing.T) {
	_ = os.Setenv("PG_QUERY_TIMEOUT", "20")
	defer func() { _ = os.Unsetenv("PG_QUERY_TIMEOUT") }()

	cfg := DefaultTimeoutConfig()
	if cfg.PGQueryTimeout != 20*time.Second {
		t.Fatalf("PGQueryTimeout from env: got %v, want 20s", cfg.PGQueryTimeout)
	}
}

func TestTimeoutConfigFromEnv_RedisConnect(t *testing.T) {
	_ = os.Setenv("REDIS_CONNECT_TIMEOUT", "7")
	defer func() { _ = os.Unsetenv("REDIS_CONNECT_TIMEOUT") }()

	cfg := DefaultTimeoutConfig()
	if cfg.RedisConnectTimeout != 7*time.Second {
		t.Fatalf("RedisConnectTimeout from env: got %v, want 7s", cfg.RedisConnectTimeout)
	}
}

func TestTimeoutConfigFromEnv_HTTPConnect(t *testing.T) {
	_ = os.Setenv("HTTP_CONNECT_TIMEOUT", "15")
	defer func() { _ = os.Unsetenv("HTTP_CONNECT_TIMEOUT") }()

	cfg := DefaultTimeoutConfig()
	if cfg.HTTPConnectTimeout != 15*time.Second {
		t.Fatalf("HTTPConnectTimeout from env: got %v, want 15s", cfg.HTTPConnectTimeout)
	}
}

func TestTimeoutConfigFromEnv_WSWrite(t *testing.T) {
	_ = os.Setenv("WS_WRITE_TIMEOUT", "5")
	defer func() { _ = os.Unsetenv("WS_WRITE_TIMEOUT") }()

	cfg := DefaultTimeoutConfig()
	if cfg.WSWriteTimeout != 5*time.Second {
		t.Fatalf("WSWriteTimeout from env: got %v, want 5s", cfg.WSWriteTimeout)
	}
}

func TestTimeoutConfigFromEnv_InvalidValue(t *testing.T) {
	_ = os.Setenv("PG_CONNECT_TIMEOUT", "not-a-number")
	defer func() { _ = os.Unsetenv("PG_CONNECT_TIMEOUT") }()

	cfg := DefaultTimeoutConfig()
	// Should fall back to default
	if cfg.PGConnectTimeout != 5*time.Second {
		t.Fatalf("invalid env value should fall back to default 5s, got %v", cfg.PGConnectTimeout)
	}
}

// ─── getDurationEnv ──────────────────────────────────────────────────

func TestGetDurationEnv_Default(t *testing.T) {
	d := getDurationEnv("NONEXISTENT_ENV_VAR_12345", 3*time.Second)
	if d != 3*time.Second {
		t.Fatalf("expected default 3s, got %v", d)
	}
}

func TestGetDurationEnv_Override(t *testing.T) {
	_ = os.Setenv("TEST_DURATION_ENV", "8")
	defer func() { _ = os.Unsetenv("TEST_DURATION_ENV") }()

	d := getDurationEnv("TEST_DURATION_ENV", 3*time.Second)
	if d != 8*time.Second {
		t.Fatalf("expected 8s from env, got %v", d)
	}
}

func TestGetDurationEnv_EmptyString(t *testing.T) {
	_ = os.Setenv("TEST_DURATION_ENV_EMPTY", "")
	defer func() { _ = os.Unsetenv("TEST_DURATION_ENV_EMPTY") }()

	d := getDurationEnv("TEST_DURATION_ENV_EMPTY", 3*time.Second)
	if d != 3*time.Second {
		t.Fatalf("empty env should use default, got %v", d)
	}
}

// ─── Benchmarks ──────────────────────────────────────────────────────

func BenchmarkDefaultTimeoutConfig(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DefaultTimeoutConfig()
	}
}
