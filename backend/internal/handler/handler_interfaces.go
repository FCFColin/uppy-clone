package handler

import (
	"context"
	"crypto/ecdsa"
	"time"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/game"
)

// UserStore is an alias for auth.UserDB — the single source of truth for user
// persistence operations. Kept as a handler-local alias so existing handler
// code (struct fields, constructor params) compiles without mass-renaming.
type UserStore = auth.UserDB

// TokenStore is an alias for auth.TokenStore — the single source of truth for
// Redis token operations.
type TokenStore = auth.TokenStore

// JWTRevocationChecker is an alias for auth.JWTRevocationChecker.
type JWTRevocationChecker = auth.JWTRevocationChecker

// ConfigStore defines config persistence operations used by handlers.
type ConfigStore interface {
	GetConfig(ctx context.Context, id string) (*domain.AppConfig, error)
	SaveConfig(ctx context.Context, c *domain.AppConfig) error
}

// LoginLockoutCache abstracts admin login-lockout Redis operations.
type LoginLockoutCache interface {
	IsLoginLocked(ctx context.Context, ip, account string) (bool, error)
	SetLoginLock(ctx context.Context, ip, account string, ttl time.Duration) error
	ResetFailedLogin(ctx context.Context, ip, account string) error
	IncrementFailedLogin(ctx context.Context, ip, account string) (int, int, error)
}

// AdminJTITracker abstracts admin-JTI tracking Redis operations.
type AdminJTITracker interface {
	GetAllAdminJTIs(ctx context.Context) ([]string, error)
	RemoveAdminJTI(ctx context.Context, jti string) error
	AddAdminJTI(ctx context.Context, jti string, ttl time.Duration) error
}

// AdminCache defines admin-specific Redis operations used by handlers.
type AdminCache interface {
	LoginLockoutCache
	IsJWTRevoked(ctx context.Context, jti string) (bool, error)
	RevokeJWT(ctx context.Context, jti string, ttl time.Duration) error
	AdminJTITracker
}

// LeaderboardStore defines leaderboard query operations used by handlers.
type LeaderboardStore interface {
	GetLeaderboard(ctx context.Context, scope string, limit int) ([]domain.LeaderboardEntry, error)
	GetUserBestScore(ctx context.Context, userID string) (int, int, error)
}

// JWTManager defines JWT signing and verification operations used by handlers.
type JWTManager interface {
	SignToken(userID, nickname string) (string, error)
	VerifyToken(tokenStr string) (userID, nickname, jti, role string, err error)
	SignWithClaims(claims map[string]any) (string, error)
	PublicKey() *ecdsa.PublicKey
}

// RefreshTokenManager defines refresh token lifecycle operations used by handlers.
type RefreshTokenManager interface {
	Generate(ctx context.Context, userID string) (string, error)
	ConsumeRefreshToken(ctx context.Context, token string) (userID string, reused bool, err error)
	Revoke(ctx context.Context, token string) error
	RevokeAllForUser(ctx context.Context, userID string) error
	RemoveFromUserSet(ctx context.Context, userID, token string) error
}

// GameService defines the game hub operations needed by handlers.
type GameService interface {
	CreateRoom(ctx context.Context) (string, error)
	CheckRoomCached(ctx context.Context, code string) (*game.RoomInfo, error)
	ListLobbiesCached(ctx context.Context, limit int, cursor string) (*domain.LobbyListResult, error)
	MatchRoom(ctx context.Context) (string, error)
	TryReserveWSConnection() bool
	DecrementWSConnection()
	WSConnCount() int64
	GetRoom(code string) game.RoomHandle
	Timeouts() config.TimeoutConfig
}
