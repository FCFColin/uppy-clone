package game

import (
	"context"

	"github.com/uppy-clone/backend/internal/domain"
)

// RoomRepository persists lobby and session state for the game aggregate.
type RoomRepository interface {
	SaveLobbyState(ctx context.Context, ls *domain.LobbyState) error
	LoadLobbyState(ctx context.Context, code string) (*domain.LobbyState, error)
	DeleteLobbyState(ctx context.Context, code string) error
	LoadAllActiveLobbies(ctx context.Context, limit int, cursor string) (*domain.LobbyListResult, error)
	CreateGameSession(ctx context.Context, gs *domain.GameSession) error
	InsertOutboxEvent(ctx context.Context, aggregateType, aggregateID string, payload []byte) error
	RecordGameResult(ctx context.Context, sessionID, roomCode string, endedAt int64, finalScore int, results []domain.GameResultPlayer) error
}

// SnapshotEncoder encodes game snapshots for persistence.
// Reserved for future use when snapshot serialization becomes injectable
// (e.g., for switching between JSON, protobuf, or compression). Currently
// only referenced from tests.
type SnapshotEncoder interface {
	Encode(state *domain.GameState) ([]byte, error)
}
