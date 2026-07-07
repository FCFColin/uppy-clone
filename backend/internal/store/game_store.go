package store

import (
	"context"

	"github.com/uppy-clone/backend/internal/domain"
)

// GameStore implements game.RoomRepository by delegating to sub-repositories.
type GameStore struct {
	lobby  *LobbyRepository
	result *ResultRepository
	outbox *OutboxRepository
}

// NewGameStore creates a GameStore that satisfies the game.RoomRepository interface.
func NewGameStore(pool pgPool) *GameStore {
	return &GameStore{
		lobby:  NewLobbyRepository(pool),
		result: NewResultRepository(pool),
		outbox: NewOutboxRepository(pool),
	}
}

func (g *GameStore) SaveLobbyState(ctx context.Context, ls *domain.LobbyState) error {
	return g.lobby.SaveLobbyState(ctx, ls)
}

func (g *GameStore) LoadLobbyState(ctx context.Context, code string) (*domain.LobbyState, error) {
	return g.lobby.LoadLobbyState(ctx, code)
}

func (g *GameStore) DeleteLobbyState(ctx context.Context, code string) error {
	return g.lobby.DeleteLobbyState(ctx, code)
}

func (g *GameStore) LoadAllActiveLobbies(ctx context.Context, limit int, cursor string) (*domain.LobbyListResult, error) {
	return g.lobby.LoadAllActiveLobbies(ctx, limit, cursor)
}

func (g *GameStore) CreateGameSession(ctx context.Context, gs *domain.GameSession) error {
	return g.result.CreateGameSession(ctx, gs)
}

func (g *GameStore) RecordGameResult(ctx context.Context, sessionID, roomCode string, endedAt int64, finalScore int, results []domain.GameResultPlayer) error {
	return g.result.RecordGameResult(ctx, sessionID, roomCode, endedAt, finalScore, results)
}

func (g *GameStore) InsertOutboxEvent(ctx context.Context, aggregateType, aggregateID string, payload []byte) error {
	return g.outbox.InsertOutboxEvent(ctx, aggregateType, aggregateID, payload)
}