package game

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/nicknames"
)

func testRNG() RNGSource {
	return newSeededRNG(42)
}

func createTestState() *domain.GameState {
	return NewGameState("TEST1", testRNG())
}

func getAllNicknameCombinations() []string {
	var out []string
	for _, adj := range nicknames.NicknameAdjectives {
		for _, cat := range nicknames.NicknameCategories {
			for _, noun := range cat {
				out = append(out, adj+noun)
			}
		}
	}
	return out
}

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

func (m *mockRoomRepository) LoadAllActiveLobbies(_ context.Context, _ int, _ string) (*domain.LobbyListResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var lobbies []domain.LobbyState
	for _, ls := range m.lobbyStates {
		copied := *ls
		lobbies = append(lobbies, copied)
	}
	return &domain.LobbyListResult{Lobbies: lobbies, Total: len(lobbies)}, nil
}

func (m *mockRoomRepository) CreateGameSession(_ context.Context, _ *domain.GameSession) error {
	return nil
}

func (m *mockRoomRepository) InsertOutboxEvent(_ context.Context, _, _ string, _ []byte) error {
	return nil
}

func (m *mockRoomRepository) RecordGameResult(_ context.Context, _, _ string, _ int64, _ int, _ []domain.GameResultPlayer) error {
	return nil
}

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

func addConnectedPlayer(r *Room, playerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state.Players[playerID] = &domain.PlayerState{
		ID:          playerID,
		PlayerIndex: len(r.state.Players),
		Nickname:    "Player" + playerID,
	}
	r.connections[playerID] = &PlayerConn{PlayerID: playerID, Send: make(chan []byte, 64)}
	r.usedNames["Player"+playerID] = true
}

func encodeTapTestPayload(x, y float32) []byte {
	buf := make([]byte, 8)
	u32 := math.Float32bits(x)
	buf[0] = byte(u32)
	buf[1] = byte(u32 >> 8)
	buf[2] = byte(u32 >> 16)
	buf[3] = byte(u32 >> 24)
	u32 = math.Float32bits(y)
	buf[4] = byte(u32)
	buf[5] = byte(u32 >> 8)
	buf[6] = byte(u32 >> 16)
	buf[7] = byte(u32 >> 24)
	return buf
}

func buildTestGameState(now int64) *domain.GameState {
	return &domain.GameState{
		Phase: domain.PhasePlaying,
		Balloon: domain.BalloonState{
			X: 0.5, Y: 0.95, VX: 0.01, VY: 0.02, Score: 100,
		},
		Bird: domain.BirdState{
			X: 0.3, Y: 0.4, VX: 0.005, VY: 0, Active: true, SpawnTimer: 0,
		},
		Ghost: domain.GhostState{
			X: 0.6, Y: 0.5, VX: -0.002, VY: 0.001, Active: true, SpawnTimer: 20, RepelTimer: 0,
		},
		Players: map[string]*domain.PlayerState{
			"p1": {
				ID: "p1", PlayerIndex: 0, Nickname: "快乐的气球",
				Palette: 1, ScoreContribution: 50, TapsCount: 10,
			},
		},
		NextPlayerIndex:     1,
		TickCount:           42,
		StartedAt:           now,
		SessionID:           "sess-123",
		LobbyCode:           "ABCDE",
		Wind:                0.3,
		WindTarget:          -0.2,
		WindChangeCountdown: 100,
		WindMicroCountdown:  5,
		WindMidCountdown:    50,
		WindMidOffset:       0.01,
		RestartVotes:        map[string]bool{"p1": true},
		RestartTimerStart:   &now,
	}
}

func assertGameStateEqual(t *testing.T, original, restored *domain.GameState) {
	t.Helper()
	if restored.Phase != original.Phase {
		t.Fatalf("Phase mismatch: got=%v, want=%v", restored.Phase, original.Phase)
	}
	if restored.Balloon.X != original.Balloon.X {
		t.Fatalf("Balloon.X mismatch: got=%v, want=%v", restored.Balloon.X, original.Balloon.X)
	}
	if restored.Balloon.Score != original.Balloon.Score {
		t.Fatalf("Balloon.Score mismatch: got=%v, want=%v", restored.Balloon.Score, original.Balloon.Score)
	}
	if restored.TickCount != original.TickCount {
		t.Fatalf("TickCount mismatch: got=%v, want=%v", restored.TickCount, original.TickCount)
	}
	if restored.SessionID != original.SessionID {
		t.Fatalf("SessionID mismatch: got=%v, want=%v", restored.SessionID, original.SessionID)
	}
	if restored.Wind != original.Wind {
		t.Fatalf("Wind mismatch: got=%v, want=%v", restored.Wind, original.Wind)
	}
	if len(restored.Players) != len(original.Players) {
		t.Fatalf("Players count mismatch: got=%d, want=%d", len(restored.Players), len(original.Players))
	}
	if restored.Players["p1"].Nickname != original.Players["p1"].Nickname {
		t.Fatalf("Player nickname mismatch: got=%v, want=%v",
			restored.Players["p1"].Nickname, original.Players["p1"].Nickname)
	}
	if len(restored.RestartVotes) != len(original.RestartVotes) {
		t.Fatalf("RestartVotes count mismatch: got=%d, want=%d", len(restored.RestartVotes), len(original.RestartVotes))
	}
}
