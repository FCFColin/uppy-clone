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
func NewGameStore(pool pgPool, deps ...Deps) *GameStore {
	d := depsOrZero(deps...)
	return &GameStore{
		lobby:  NewLobbyRepository(pool, d),
		result: NewResultRepository(pool, d),
		outbox: NewOutboxRepository(pool, d),
	}
}

// SaveLobbyState persists the lobby state to the database.
func (g *GameStore) SaveLobbyState(ctx context.Context, ls *domain.LobbyState) error {
	return g.lobby.SaveLobbyState(ctx, ls)
}

// LoadLobbyState loads the lobby state by room code.
func (g *GameStore) LoadLobbyState(ctx context.Context, code string) (*domain.LobbyState, error) {
	return g.lobby.LoadLobbyState(ctx, code)
}

// DeleteLobbyState removes the lobby state for the given room code.
func (g *GameStore) DeleteLobbyState(ctx context.Context, code string) error {
	return g.lobby.DeleteLobbyState(ctx, code)
}

// LoadAllActiveLobbies returns a paginated list of active lobbies.
func (g *GameStore) LoadAllActiveLobbies(ctx context.Context, limit int, cursor string) (*domain.LobbyListResult, error) {
	return g.lobby.LoadAllActiveLobbies(ctx, limit, cursor)
}

// CreateGameSession inserts a new game session record.
func (g *GameStore) CreateGameSession(ctx context.Context, gs *domain.GameSession) error {
	return g.result.CreateGameSession(ctx, gs)
}

// RecordGameResult records the final results of a game session.
func (g *GameStore) RecordGameResult(ctx context.Context, sessionID, roomCode string, endedAt int64, finalScore int, results []domain.GameResultPlayer) error {
	return g.result.RecordGameResult(ctx, sessionID, roomCode, endedAt, finalScore, results)
}

// InsertOutboxEvent inserts an outbox event for the transactional outbox pattern.
func (g *GameStore) InsertOutboxEvent(ctx context.Context, aggregateType, aggregateID string, payload []byte) error {
	return g.outbox.InsertOutboxEvent(ctx, aggregateType, aggregateID, payload)
}
