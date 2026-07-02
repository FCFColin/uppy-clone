package auth

import (
	"context"
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
	refreshTokenExpiry     = 7 * 24 * time.Hour // 7 days
	refreshTokenPrefix     = "refresh_token:"
	refreshTokenReusePrefix = "refresh_token:reuse:" // reuse detection marker
	userTokensSetPrefix    = "refresh_tokens:user:"  // reverse index for efficient revocation
)

// consumeRefreshTokenScript atomically validates, consumes, and detects reuse
// of a refresh token.
// KEYS[1] = refresh_token:<token>
// KEYS[2] = refresh_token:reuse:<token>
// ARGV[1] = reuse marker TTL (seconds, same as refresh token TTL)
// Returns:
//   {1, userID}  = token valid and consumed
//   {0, userID}  = reuse detected (token already consumed)
//   {-1}         = token not found (never existed or expired)
var consumeRefreshTokenScript = redis.NewScript(`
	local userID = redis.call('GET', KEYS[1])
	if userID then
		redis.call('DEL', KEYS[1])
		redis.call('SET', KEYS[2], userID, 'EX', ARGV[1])
		return {1, userID}
	end
	local reuseUserID = redis.call('GET', KEYS[2])
	if reuseUserID then
		return {0, reuseUserID}
	end
	return {-1}
`)

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

var errRefreshTokenReused = fmt.Errorf("refresh token reuse detected")

// ConsumeRefreshTokenResult describes the outcome of an atomic consume.
type ConsumeRefreshTokenResult struct {
	UserID string
	Reused bool // true if this token was already consumed (reuse detected)
}

// ConsumeRefreshToken atomically validates and consumes a refresh token.
// Returns the userID, a reuse flag, and an error.
// When reuse is detected, the caller should revoke ALL tokens for the user.
func (m *RefreshTokenManager) ConsumeRefreshToken(ctx context.Context, token string) (*ConsumeRefreshTokenResult, error) {
	key := refreshTokenPrefix + token
	reuseKey := refreshTokenReusePrefix + token
	result, err := consumeRefreshTokenScript.Run(ctx, m.rdb,
		[]string{key, reuseKey},
		int(refreshTokenExpiry.Seconds())).Result()
	if err != nil {
		return nil, fmt.Errorf("consume refresh token: %w", err)
	}
	vals, ok := result.([]interface{})
	if !ok || len(vals) < 1 {
		return nil, fmt.Errorf("consume refresh token: unexpected result")
	}
	status, ok := vals[0].(int64)
	if !ok {
		return nil, fmt.Errorf("consume refresh token: unexpected status type")
	}
	switch status {
	case -1:
		return nil, fmt.Errorf("invalid or expired refresh token")
	case 0:
		if len(vals) < 2 {
			return nil, fmt.Errorf("consume refresh token: unexpected reuse result")
		}
		userID, ok := vals[1].(string)
		if !ok || userID == "" {
			return nil, fmt.Errorf("consume refresh token: unexpected userID type")
		}
		return &ConsumeRefreshTokenResult{UserID: userID, Reused: true}, nil
	case 1:
		if len(vals) < 2 {
			return nil, fmt.Errorf("consume refresh token: unexpected success result")
		}
		userID, ok := vals[1].(string)
		if !ok || userID == "" {
			return nil, fmt.Errorf("consume refresh token: unexpected userID type")
		}
		return &ConsumeRefreshTokenResult{UserID: userID, Reused: false}, nil
	default:
		return nil, fmt.Errorf("consume refresh token: unknown status %d", status)
	}
}

// RemoveFromUserSet removes a specific token from the user's reverse-index set.
// Called after ConsumeRefreshToken to keep the reverse index consistent.
func (m *RefreshTokenManager) RemoveFromUserSet(ctx context.Context, userID, token string) error {
	userTokensKey := userTokensSetPrefix + userID
	return m.rdb.SRem(ctx, userTokensKey, token).Err()
}

// Validate checks if a refresh token is valid and returns the associated userID.
// Deprecated: Use ConsumeRefreshToken for atomic validate+consume.
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
	if _, err := randRead(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
