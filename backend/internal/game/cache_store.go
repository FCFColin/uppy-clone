package game

import (
	"context"
	"time"

	"github.com/uppy-clone/backend/internal/domain"
)

// CacheStore defines Redis-backed caching operations needed by the game engine.
type CacheStore interface {
	GetRoomRegistry(ctx context.Context, code string) (*domain.RoomRegistryInfo, error)
	RegisterRoom(ctx context.Context, code string, data []byte, ttl time.Duration) error
	UnregisterRoom(ctx context.Context, code string) error
	GetCachedLobbyList(ctx context.Context, limit int, cursor string) ([]byte, bool, error)
	SetCachedLobbyList(ctx context.Context, limit int, cursor string, data []byte) error
	GetCachedRoomCheck(ctx context.Context, code string) ([]byte, bool, error)
	SetCachedRoomCheck(ctx context.Context, code string, data []byte) error
	InvalidateLobbyListCaches(ctx context.Context) error
	InvalidateRoomCheck(ctx context.Context, code string) error
	EnqueueGameResult(ctx context.Context, payload []byte) error
}
