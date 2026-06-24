package auth

// 企业为何需要：安全关键组件（中间件/认证/管理）零测试是最高风险——任何改动都可在生产暴露。

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

const testUserID = "user-123"

// We test RefreshTokenManager using a miniredis in-memory Redis server
// if available, or test the pure logic functions.
// Since the manager uses concrete *redis.Client, we test what we can
// without requiring a real Redis connection.

func TestRefreshTokenManager_GenerateSecureToken(t *testing.T) {
	t.Run("generateSecureToken produces hex string", func(t *testing.T) {
		token, err := generateSecureToken(32)
		if err != nil {
			t.Fatalf("generateSecureToken error: %v", err)
		}
		// 32 bytes = 64 hex chars
		if len(token) != 64 {
			t.Errorf("token length = %d, want 64", len(token))
		}
	})

	t.Run("generateSecureToken produces unique tokens", func(t *testing.T) {
		token1, _ := generateSecureToken(32)
		token2, _ := generateSecureToken(32)
		if token1 == token2 {
			t.Error("two generated tokens should not be equal")
		}
	})
}

func TestRefreshTokenManager_Constants(t *testing.T) {
	t.Run("refresh token expiry is 7 days", func(t *testing.T) {
		if refreshTokenExpiry != 7*24*time.Hour {
			t.Errorf("refreshTokenExpiry = %v, want 7 days", refreshTokenExpiry)
		}
	})

	t.Run("refresh token prefix is correct", func(t *testing.T) {
		if refreshTokenPrefix != "refresh_token:" {
			t.Errorf("refreshTokenPrefix = %q, want %q", refreshTokenPrefix, "refresh_token:")
		}
	})
}

func TestNewRefreshTokenManager(t *testing.T) {
	t.Run("creates manager with nil client", func(t *testing.T) {
		// Creating with nil client is valid; methods will panic if called
		mgr := NewRefreshTokenManager(nil)
		if mgr == nil {
			t.Error("NewRefreshTokenManager should return non-nil manager")
		}
	})
}

// The following tests require a real Redis connection.
// They are skipped if Redis is not available.
// In CI, use testcontainers or miniredis.

func skipIfNoRedis(t *testing.T, rdb *redis.Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping integration test")
	}
}

func TestRefreshTokenManager_Integration(t *testing.T) {
	// Try to connect to local Redis
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	skipIfNoRedis(t, rdb)
	defer rdb.Close()

	mgr := NewRefreshTokenManager(rdb)
	ctx := context.Background()

	t.Run("Generate creates token in Redis", func(t *testing.T) {
		testRefreshTokenGenerate(t, mgr, rdb, ctx)
	})
	t.Run("Validate accepts valid token", func(t *testing.T) {
		testRefreshTokenValidate(t, mgr, ctx)
	})
	t.Run("Validate rejects invalid token", func(t *testing.T) {
		testRefreshTokenValidateInvalid(t, mgr, ctx)
	})
	t.Run("Revoke removes token", func(t *testing.T) {
		testRefreshTokenRevoke(t, mgr, ctx)
	})
	t.Run("RevokeAllForUser removes all tokens for a user", func(t *testing.T) {
		testRefreshTokenRevokeAll(t, mgr, ctx)
	})
}

func testRefreshTokenGenerate(t *testing.T, mgr *RefreshTokenManager, rdb *redis.Client, ctx context.Context) {
	token, err := mgr.Generate(ctx, testUserID)
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	if token == "" {
		t.Error("Generate should return non-empty token")
	}

	key := refreshTokenPrefix + token
	val, err := rdb.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("token not found in Redis: %v", err)
	}
	if val != testUserID {
		t.Errorf("token value = %q, want %q", val, testUserID)
	}
}

func testRefreshTokenValidate(t *testing.T, mgr *RefreshTokenManager, ctx context.Context) {
	token, err := mgr.Generate(ctx, "user-validate")
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	userID, err := mgr.Validate(ctx, token)
	if err != nil {
		t.Fatalf("Validate error: %v", err)
	}
	if userID != "user-validate" {
		t.Errorf("userID = %q, want %q", userID, "user-validate")
	}
}

func testRefreshTokenValidateInvalid(t *testing.T, mgr *RefreshTokenManager, ctx context.Context) {
	_, err := mgr.Validate(ctx, "nonexistent-token")
	if err == nil {
		t.Error("Validate should return error for invalid token")
	}
}

func testRefreshTokenRevoke(t *testing.T, mgr *RefreshTokenManager, ctx context.Context) {
	token, err := mgr.Generate(ctx, "user-revoke")
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	err = mgr.Revoke(ctx, token)
	if err != nil {
		t.Fatalf("Revoke error: %v", err)
	}

	_, err = mgr.Validate(ctx, token)
	if err == nil {
		t.Error("Validate should fail after Revoke")
	}
}

func testRefreshTokenRevokeAll(t *testing.T, mgr *RefreshTokenManager, ctx context.Context) {
	token1, _ := mgr.Generate(ctx, "user-revokeall")
	token2, _ := mgr.Generate(ctx, "user-revokeall")

	err := mgr.RevokeAllForUser(ctx, "user-revokeall")
	if err != nil {
		t.Fatalf("RevokeAllForUser error: %v", err)
	}

	_, err1 := mgr.Validate(ctx, token1)
	_, err2 := mgr.Validate(ctx, token2)
	if err1 == nil || err2 == nil {
		t.Error("both tokens should be invalid after RevokeAllForUser")
	}
}
