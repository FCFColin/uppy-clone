package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/domain"
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
		return nil, domain.ErrNotFound
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
	exportData["game_results"] = []interface{}{}
	if results, err := dataStore.GetGameResultsByUserID(ctx, userID); err == nil && results != nil {
		exportData["game_results"] = results
	}
	return exportData, nil
}

// DeleteUserData revokes sessions and anonymizes PII for GDPR erasure.
func DeleteUserData(ctx context.Context, jwtMgr *JWTManager, refreshMgr *RefreshTokenManager, tokens TokenStore, dataStore UserDataStore, userID string, r *http.Request) error {
	if refreshMgr != nil {
		_ = refreshMgr.RevokeAllForUser(ctx, userID)
	}
	RevokeAllTokens(ctx, jwtMgr, refreshMgr, tokens, r)
	if dataStore != nil {
		if err := dataStore.AnonymizeUser(ctx, userID); err != nil {
			return fmt.Errorf("anonymize user: %w", err)
		}
	}
	audit.Log(ctx, audit.AuditEntry{
		Action:   "gdpr_hard_delete",
		ActorID:  userID,
		Resource: "user/" + userID,
	})
	return nil
}

// RefreshSessionResult holds rotated tokens from refresh flow.
type RefreshSessionResult struct {
	AccessToken  string
	RefreshToken string
}

// RefreshSession validates and rotates refresh tokens atomically,
// detecting token reuse (theft) and revoking all tokens for the compromised user.
func RefreshSession(ctx context.Context, refreshMgr *RefreshTokenManager, jwtMgr *JWTManager, dataStore UserDataStore, oldToken string) (*RefreshSessionResult, error) {
	result, err := refreshMgr.ConsumeRefreshToken(ctx, oldToken)
	if err != nil {
		return nil, fmt.Errorf("consume refresh token: %w", err)
	}

	if result.Reused {
		slog.Warn("refresh token reuse detected — revoking all tokens for user",
			"user_id", result.UserID)
		if revokeErr := refreshMgr.RevokeAllForUser(ctx, result.UserID); revokeErr != nil {
			slog.Error("failed to revoke all tokens after reuse detection",
				"user_id", result.UserID, "error", revokeErr)
		}
		return nil, fmt.Errorf("refresh token has already been used")
	}

	user, err := dataStore.GetUserByID(ctx, result.UserID)
	if err != nil || user == nil {
		return nil, fmt.Errorf("user not found")
	}

	_ = refreshMgr.RemoveFromUserSet(ctx, result.UserID, oldToken)

	accessToken, err := jwtMgr.SignToken(result.UserID, user.Nickname)
	if err != nil {
		return nil, fmt.Errorf("sign token: %w", err)
	}
	newRefresh, err := refreshMgr.Generate(ctx, result.UserID)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	return &RefreshSessionResult{AccessToken: accessToken, RefreshToken: newRefresh}, nil
}


