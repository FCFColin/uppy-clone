package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/game"
)

// UserStore defines user persistence operations used by handlers.
type UserStore interface {
	GetUserByID(ctx context.Context, id string) (*domain.User, error)
	CreateUser(ctx context.Context, user *domain.User) error
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	UpdateUserLastLogin(ctx context.Context, id string) error
	AnonymizeUser(ctx context.Context, id string) error
	GetGameResultsByUserID(ctx context.Context, userID string) ([]domain.GameResult, error)
}

// TokenStore defines token/Redis operations used by handlers.
type TokenStore interface {
	StoreMagicToken(ctx context.Context, hashedToken string, data []byte, ttl time.Duration) error
	ConsumeMagicToken(ctx context.Context, tokenHash string) ([]byte, error)
	DeleteMagicToken(ctx context.Context, hashedToken string) error
	CheckRateLimit(ctx context.Context, key string, maxCount int64, window time.Duration) (bool, error)
	IsJWTRevoked(ctx context.Context, jti string) (bool, error)
	RevokeJWT(ctx context.Context, jti string, ttl time.Duration) error
	EnqueueEmail(ctx context.Context, payload []byte) error
}

// ConfigStore defines config persistence operations used by handlers.
type ConfigStore interface {
	GetConfig(ctx context.Context, id string) (*domain.AppConfig, error)
	SaveConfig(ctx context.Context, c *domain.AppConfig) error
}

// AdminCache defines admin-specific Redis operations used by handlers.
type AdminCache interface {
	IsLoginLocked(ctx context.Context, ip, account string) (bool, error)
	SetLoginLock(ctx context.Context, ip, account string, ttl time.Duration) error
	ResetFailedLogin(ctx context.Context, ip, account string) error
	IncrementFailedLogin(ctx context.Context, ip, account string) (int, int, error)
	IsJWTRevoked(ctx context.Context, jti string) (bool, error)
	RevokeJWT(ctx context.Context, jti string, ttl time.Duration) error
	GetAllAdminJTIs(ctx context.Context) ([]string, error)
	RemoveAdminJTI(ctx context.Context, jti string) error
	AddAdminJTI(ctx context.Context, jti string, ttl time.Duration) error
}

// LeaderboardStore defines leaderboard query operations used by handlers.
type LeaderboardStore interface {
	GetLeaderboard(ctx context.Context, scope string, limit int) ([]domain.LeaderboardEntry, error)
	GetUserBestScore(ctx context.Context, userID string) (int, int, error)
}

// JWTManager defines JWT signing and verification operations used by handlers.
type JWTManager interface {
	SignToken(userID, nickname string) (string, error)
	VerifyToken(tokenStr string) (userID, nickname, jti string, err error)
	Secret() []byte
}

// RefreshTokenManager defines refresh token lifecycle operations used by handlers.
type RefreshTokenManager interface {
	Generate(ctx context.Context, userID string) (string, error)
	ConsumeRefreshToken(ctx context.Context, token string) (userID string, reused bool, err error)
	Revoke(ctx context.Context, token string) error
	RevokeAllForUser(ctx context.Context, userID string) error
	RemoveFromUserSet(ctx context.Context, userID, token string) error
}

// JWTRevocationChecker checks if a JWT has been revoked by its jti.
type JWTRevocationChecker interface {
	IsJWTRevoked(ctx context.Context, jti string) (bool, error)
}

// AuthService defines the auth operations needed by handlers.
type AuthService interface {
	RequestMagicLink(ctx context.Context, email string, r *http.Request) error
	RefreshSession(ctx context.Context, refreshToken string, r *http.Request) (accessToken, newRefreshToken string, cookieMaxAge int, err error)
	VerifyMagicLink(ctx context.Context, token string, r *http.Request) (userID, accessToken, refreshToken string, err error)
	QuickPlay(ctx context.Context, nickname string, r *http.Request) (userID, accessToken, refreshToken string, err error)
	ExportUserData(ctx context.Context, userID string) (*domain.User, []domain.GameResult, error)
	DeleteUserData(ctx context.Context, userID string, r *http.Request) error
	RevokeRefreshToken(ctx context.Context, token string) error
	RevokeAllTokens(ctx context.Context, r *http.Request) error
	AuthenticatedUserFromRequest(r *http.Request) (userID, nickname string, ok bool)
	GetJTI(r *http.Request) string
	IsJWTRevoked(ctx context.Context, jti string) (bool, error)
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
	GetRoom(code string) *game.Room
	Timeouts() config.TimeoutConfig
}
