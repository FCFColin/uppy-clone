package auth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/store"
)

// UserDataStore abstracts GDPR user data operations.
type UserDataStore interface {
	GetUserByID(ctx context.Context, id string) (*domain.User, error)
	AnonymizeUser(ctx context.Context, id string) error
	GetGameResultsByUserID(ctx context.Context, userID string) ([]domain.GameResult, error)
}

// ExportUserData builds the GDPR export payload for a user.
func ExportUserData(ctx context.Context, dataStore UserDataStore, userID string) (map[string]interface{}, error) {
	user, err := dataStore.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	exportData := map[string]interface{}{
		"user": map[string]interface{}{
			"id":         user.ID,
			"email":      user.Email,
			"nickname":   user.Nickname,
			"created_at": user.CreatedAt,
			"last_login": user.LastLogin,
		},
	}
	if results, err := dataStore.GetGameResultsByUserID(ctx, userID); err == nil && results != nil {
		exportData["game_results"] = results
	} else {
		exportData["game_results"] = []interface{}{}
	}
	return exportData, nil
}

// DeleteUserData revokes sessions and anonymizes PII for GDPR erasure.
func DeleteUserData(ctx context.Context, jwtMgr *JWTManager, refreshMgr *RefreshTokenManager, redis *store.RedisStore, dataStore UserDataStore, userID string, r *http.Request) error {
	if refreshMgr != nil {
		_ = refreshMgr.RevokeAllForUser(ctx, userID)
	}
	RevokeAllTokens(ctx, jwtMgr, refreshMgr, redis, r)
	if dataStore != nil {
		if err := dataStore.AnonymizeUser(ctx, userID); err != nil {
			return fmt.Errorf("anonymize user: %w", err)
		}
	}
	return nil
}

// RefreshSessionResult holds rotated tokens from refresh flow.
type RefreshSessionResult struct {
	AccessToken  string
	RefreshToken string
}

// RefreshSession validates and rotates refresh tokens.
func RefreshSession(ctx context.Context, refreshMgr *RefreshTokenManager, jwtMgr *JWTManager, dataStore UserDataStore, oldToken string) (*RefreshSessionResult, error) {
	userID, err := refreshMgr.Validate(ctx, oldToken)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}
	user, err := dataStore.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		return nil, fmt.Errorf("user not found")
	}
	_ = refreshMgr.Revoke(ctx, oldToken)
	accessToken, err := jwtMgr.SignToken(userID, user.Nickname)
	if err != nil {
		return nil, fmt.Errorf("sign token: %w", err)
	}
	newRefresh, err := refreshMgr.Generate(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	return &RefreshSessionResult{AccessToken: accessToken, RefreshToken: newRefresh}, nil
}
