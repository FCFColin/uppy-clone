package auth

import (
	"context"
	"time"

	"github.com/uppy-clone/backend/internal/domain"
)

// UserDB abstracts PostgreSQL user operations used by auth.
type UserDB interface {
	GetUserByID(ctx context.Context, id string) (*domain.User, error)
	CreateUser(ctx context.Context, user *domain.User) error
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	UpdateUserLastLogin(ctx context.Context, id string) error
	AnonymizeUser(ctx context.Context, id string) error
	GetGameResultsByUserID(ctx context.Context, userID string) ([]domain.GameResult, error)
}

// TokenStore abstracts Redis token operations used by auth.
type TokenStore interface {
	StoreMagicToken(ctx context.Context, hashedToken string, data []byte, ttl time.Duration) error
	ConsumeMagicToken(ctx context.Context, tokenHash string) ([]byte, error)
	DeleteMagicToken(ctx context.Context, hashedToken string) error
	CheckRateLimit(ctx context.Context, key string, maxCount int64, window time.Duration) (bool, error)
	IsJWTRevoked(ctx context.Context, jti string) (bool, error)
	RevokeJWT(ctx context.Context, jti string, ttl time.Duration) error
	EnqueueEmail(ctx context.Context, payload []byte) error
}
