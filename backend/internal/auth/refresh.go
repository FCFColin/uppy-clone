package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// Enterprise rationale: Long-lived access tokens are a security risk — if leaked,
// they grant access until expiry. The dual-token pattern (access + refresh):
//   - Access token: short-lived (15min), used for API calls
//   - Refresh token: long-lived (7d), stored in Redis, used only to get new access tokens
//   - Refresh tokens can be revoked by deleting from Redis
//
// This limits the damage window of a leaked access token to 15 minutes.
// Trade-off: Extra Redis round-trip for refresh, but security benefit outweighs cost.

const (
	refreshTokenExpiry  = 7 * 24 * time.Hour // 7 days
	refreshTokenPrefix  = "refresh_token:"
	userTokensSetPrefix = "refresh_tokens:user:" // reverse index for efficient revocation
)

// RefreshTokenManager handles refresh token lifecycle.
type RefreshTokenManager struct {
	rdb *redis.Client
}

// NewRefreshTokenManager creates a new manager backed by Redis.
func NewRefreshTokenManager(rdb *redis.Client) *RefreshTokenManager {
	return &RefreshTokenManager{rdb: rdb}
}

// Generate creates a new refresh token and stores it in Redis.
func (m *RefreshTokenManager) Generate(ctx context.Context, userID string) (string, error) {
	token, err := generateSecureToken(32)
	if err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}

	key := refreshTokenPrefix + token
	if err := m.rdb.Set(ctx, key, userID, refreshTokenExpiry).Err(); err != nil {
		return "", fmt.Errorf("store refresh token: %w", err)
	}

	// Track token in user's token set for efficient revocation (N+1 fix).
	// 企业为何需要：RevokeAllForUser 原 SCAN 全键空间 O(N) 复杂度，反向索引 Set 降为 O(K)。
	userTokensKey := userTokensSetPrefix + userID
	m.rdb.SAdd(ctx, userTokensKey, token)
	m.rdb.Expire(ctx, userTokensKey, refreshTokenExpiry)

	slog.Info("refresh token generated", "user_id", userID)
	return token, nil
}

// Validate checks if a refresh token is valid and returns the associated userID.
func (m *RefreshTokenManager) Validate(ctx context.Context, token string) (string, error) {
	key := refreshTokenPrefix + token
	userID, err := m.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("invalid or expired refresh token")
	}
	if err != nil {
		return "", fmt.Errorf("validate refresh token: %w", err)
	}
	return userID, nil
}

// Revoke deletes a refresh token from Redis.
func (m *RefreshTokenManager) Revoke(ctx context.Context, token string) error {
	key := refreshTokenPrefix + token
	return m.rdb.Del(ctx, key).Err()
}

// RevokeAllForUser removes all refresh tokens for a user.
// This is used when a user changes password or is compromised.
// 企业为何需要：原实现 SCAN 全键空间 O(N)，反向索引 Set 降为 O(K)（K=用户 token 数）。
func (m *RefreshTokenManager) RevokeAllForUser(ctx context.Context, userID string) error {
	userTokensKey := userTokensSetPrefix + userID
	// Get all tokens for this user from the reverse-index set (N+1 fix).
	tokens, err := m.rdb.SMembers(ctx, userTokensKey).Result()
	if err != nil {
		return fmt.Errorf("get user tokens: %w", err)
	}

	// Delete each token key and the set key.
	for _, token := range tokens {
		m.rdb.Del(ctx, refreshTokenPrefix+token)
	}
	m.rdb.Del(ctx, userTokensKey)

	slog.Info("all refresh tokens revoked for user", "user_id", userID, "count", len(tokens))
	return nil
}

func generateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
