package game

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/store"
)

// mockBroadcaster 是基于内存的 Broadcaster 实现，用于单元测试（无需真实 Redis）。
type mockBroadcaster struct {
	mu        sync.Mutex
	handlers  map[string]func(BroadcastMessage)
	published []BroadcastMessage
	closed    bool
}

func newMockBroadcaster() *mockBroadcaster {
	return &mockBroadcaster{handlers: make(map[string]func(BroadcastMessage))}
}

func (m *mockBroadcaster) Publish(_ context.Context, roomCode string, msg BroadcastMessage) error {
	m.mu.Lock()
	m.published = append(m.published, msg)
	handler := m.handlers[roomCode]
	m.mu.Unlock()
	if handler != nil {
		handler(msg)
	}
	return nil
}

func (m *mockBroadcaster) Subscribe(roomCode string, handler func(BroadcastMessage)) (func(), error) {
	m.mu.Lock()
	m.handlers[roomCode] = handler
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		delete(m.handlers, roomCode)
		m.mu.Unlock()
	}, nil
}

func (m *mockBroadcaster) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.handlers = make(map[string]func(BroadcastMessage))
	return nil
}

// mockRoomRepository is an in-memory implementation of RoomRepository for testing.
type mockRoomRepository struct {
	mu          sync.RWMutex
	lobbyStates map[string]*domain.LobbyState
	saveErr     error
	loadErr     error
	deleteErr   error
	saveCount   int
	loadCount   int
	deleteCount int
}

func newMockRoomRepository() *mockRoomRepository {
	return &mockRoomRepository{
		lobbyStates: make(map[string]*domain.LobbyState),
	}
}

func (m *mockRoomRepository) SaveLobbyState(_ context.Context, ls *domain.LobbyState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saveCount++
	if m.saveErr != nil {
		return m.saveErr
	}
	if ls == nil {
		return errors.New("lobby state is nil")
	}
	stored := *ls
	m.lobbyStates[ls.Code] = &stored
	return nil
}

func (m *mockRoomRepository) LoadLobbyState(_ context.Context, code string) (*domain.LobbyState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loadCount++
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	ls, ok := m.lobbyStates[code]
	if !ok {
		return nil, fmt.Errorf("lobby state not found: %s", code)
	}
	copied := *ls
	return &copied, nil
}

func (m *mockRoomRepository) DeleteLobbyState(_ context.Context, code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteCount++
	if m.deleteErr != nil {
		return m.deleteErr
	}
	if _, ok := m.lobbyStates[code]; !ok {
		return fmt.Errorf("lobby state not found: %s", code)
	}
	delete(m.lobbyStates, code)
	return nil
}

func (m *mockRoomRepository) LoadAllActiveLobbies(_ context.Context, _ int, _ string) (*store.LobbyListResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var lobbies []domain.LobbyState
	for _, ls := range m.lobbyStates {
		copied := *ls
		lobbies = append(lobbies, copied)
	}
	return &store.LobbyListResult{Lobbies: lobbies, Total: len(lobbies)}, nil
}

func (m *mockRoomRepository) CreateGameSession(_ context.Context, _ *domain.GameSession) error {
	return nil
}

func (m *mockRoomRepository) InsertOutboxEvent(_ context.Context, _, _ string, _ []byte) error {
	return nil
}

// mockSnapshotEncoder is a test implementation of SnapshotEncoder.
type mockSnapshotEncoder struct {
	encodeErr   error
	encodeCount int
	lastState   *domain.GameState
}

func (m *mockSnapshotEncoder) Encode(state *domain.GameState) ([]byte, error) {
	m.encodeCount++
	m.lastState = state
	if m.encodeErr != nil {
		return nil, m.encodeErr
	}
	if state == nil {
		return []byte("null"), nil
	}
	return []byte(fmt.Sprintf(`{"phase":"%s","lobbyCode":"%s"}`, state.Phase, state.LobbyCode)), nil
}
