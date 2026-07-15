package game

import (
	"math"
	"testing"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

func TestNewGameState_InitialValues(t *testing.T) {
	state := NewGameState("TEST", 42, testRNG())

	if state.Phase != domain.PhaseWaiting {
		t.Fatalf("初始 phase 应为 waiting，got=%v", state.Phase)
	}
	if state.TickCount != 0 {
		t.Fatalf("初始 tickCount 应为 0，got=%d", state.TickCount)
	}
	if state.Balloon.X != 0.5 {
		t.Fatalf("初始气球 X 应为 0.5，got=%v", state.Balloon.X)
	}
	if state.Balloon.Y != 0.95 {
		t.Fatalf("初始气球 Y 应为 0.95，got=%v", state.Balloon.Y)
	}
	if state.Balloon.VX != 0 || state.Balloon.VY != 0 {
		t.Fatalf("初始气球速度应为 0，got VX=%v VY=%v", state.Balloon.VX, state.Balloon.VY)
	}
	if state.Balloon.Score != 0 {
		t.Fatalf("初始分数应为 0，got=%d", state.Balloon.Score)
	}
	if !state.Ghost.Active {
		t.Fatal("初始幽灵应已激活")
	}
	if state.Wind == 0 {
		t.Fatalf("初始风场不应为 0（游戏开始即应有风），got=%v", state.Wind)
	}
	if state.WindTarget == 0 {
		t.Fatalf("初始 WindTarget 不应为 0，got=%v", state.WindTarget)
	}
	if state.WindChangeCountdown != 112 {
		t.Fatalf("初始 WindChangeCountdown 应为 112，got=%d", state.WindChangeCountdown)
	}
	if len(state.RestartVotes) != 0 {
		t.Fatalf("初始 RestartVotes 应为空，got=%d", len(state.RestartVotes))
	}
	if state.RestartTimerStart != nil {
		t.Fatal("初始 RestartTimerStart 应为 nil")
	}
	if string(state.LobbyCode) != "TEST" {
		t.Fatalf("LobbyCode 应为 TEST，got=%v", string(state.LobbyCode))
	}
}

func TestNewGameState_GhostInBounds(t *testing.T) {
	for i := 0; i < 50; i++ {
		state := NewGameState("TEST", int64(i), newSeededRNG(int64(i)))

		if state.Ghost.X < 0.15 || state.Ghost.X > 0.85 {
			t.Fatalf("幽灵 X 应在 0.15-0.85，got=%v", state.Ghost.X)
		}
		if state.Ghost.Y < 0.3 || state.Ghost.Y > 0.75 {
			t.Fatalf("幽灵 Y 应在 0.3-0.75，got=%v", state.Ghost.Y)
		}
	}
}

func TestNewGameState_GhostHasSpeed(t *testing.T) {
	state := NewGameState("TEST", 42, testRNG())
	speed := math.Sqrt(state.Ghost.VX*state.Ghost.VX + state.Ghost.VY*state.Ghost.VY)
	if speed == 0 {
		t.Fatal("初始幽灵应有非零速度")
	}
}

func TestInitialWind_Clamp(t *testing.T) {
	wind, target := initialWind(testRNG())
	if wind > protocol.WindClamp || wind < -protocol.WindClamp {
		t.Fatalf("initialWind wind = %v, should be clamped", wind)
	}
	if target == 0 {
		t.Fatal("windTarget should be non-zero with random wind")
	}
	_ = wind
}

// ─── ResetGameEntities ───────────────────────────────────────────────

func TestResetGameEntities_ResetsBalloon(t *testing.T) {
	state := createTestState()
	state.Balloon.X = 0.3
	state.Balloon.Y = 0.5
	state.Balloon.VX = 0.1
	state.Balloon.VY = 0.2
	state.Balloon.Score = 500
	state.TickCount = 200

	ResetGameEntities(state, RandomSpawnTimer(testRNG()), testRNG())

	if state.Balloon.X != 0.5 {
		t.Fatalf("重置后气球 X 应为 0.5，got=%v", state.Balloon.X)
	}
	if state.Balloon.Y != 0.95 {
		t.Fatalf("重置后气球 Y 应为 0.95，got=%v", state.Balloon.Y)
	}
	if state.Balloon.VX != 0 || state.Balloon.VY != 0 {
		t.Fatalf("重置后气球速度应为 0，got VX=%v VY=%v", state.Balloon.VX, state.Balloon.VY)
	}
	if state.Balloon.Score != 0 {
		t.Fatalf("重置后分数应为 0，got=%d", state.Balloon.Score)
	}
	if state.TickCount != 0 {
		t.Fatalf("重置后 tickCount 应为 0，got=%d", state.TickCount)
	}
}

func TestResetGameEntities_ResetsGhost(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = false
	state.Ghost.SpawnTimer = 100

	ResetGameEntities(state, RandomSpawnTimer(testRNG()), testRNG())

	if !state.Ghost.Active {
		t.Fatal("重置后幽灵应已激活")
	}
}

func TestResetGameEntities_ResetsWind(t *testing.T) {
	state := createTestState()
	state.Wind = 0.8
	state.WindTarget = -0.5
	state.WindChangeCountdown = 10

	ResetGameEntities(state, RandomSpawnTimer(testRNG()), testRNG())

	if state.Wind == 0 {
		t.Fatalf("重置后风场不应为 0（游戏开始即应有风），got=%v", state.Wind)
	}
	if state.WindTarget == 0 {
		t.Fatalf("重置后 WindTarget 不应为 0，got=%v", state.WindTarget)
	}
	if state.WindChangeCountdown != 112 {
		t.Fatalf("重置后 WindChangeCountdown 应为 112，got=%d", state.WindChangeCountdown)
	}
}

func TestResetGameEntities_ResetsVotes(t *testing.T) {
	state := createTestState()
	state.RestartVotes["player1"] = true
	state.RestartVotes["player2"] = true
	now := int64(1234567890)
	state.RestartTimerStart = &now

	ResetGameEntities(state, RandomSpawnTimer(testRNG()), testRNG())

	if len(state.RestartVotes) != 0 {
		t.Fatalf("重置后 RestartVotes 应为空，got=%d", len(state.RestartVotes))
	}
	if state.RestartTimerStart != nil {
		t.Fatal("重置后 RestartTimerStart 应为 nil")
	}
}

// ─── SerializeState / DeserializeState ───────────────────────────────

func TestDeserializeState_InvalidJSON(t *testing.T) {
	_, err := DeserializeState([]byte("{invalid"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestDeserializeState_InitializesNilMaps(t *testing.T) {
	data := []byte(`{"lobbyCode":"MAPS1","phase":"waiting"}`)
	state, err := DeserializeState(data)
	if err != nil {
		t.Fatalf("DeserializeState: %v", err)
	}
	if state.Players == nil {
		t.Fatal("Players map should be initialized")
	}
	if state.RestartVotes == nil {
		t.Fatal("RestartVotes map should be initialized")
	}
	if string(state.LobbyCode) != "MAPS1" {
		t.Fatalf("LobbyCode = %q, want MAPS1", string(state.LobbyCode))
	}
}

func TestSerializeDeserialize_RoundTrip(t *testing.T) {
	original := buildTestGameState(1700000000)

	data, err := SerializeState(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	restored, err := DeserializeState(data)
	if err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	assertGameStateEqual(t, original, restored)
}

// buildTestGameState constructs a GameState with representative fields for round-trip testing.

func BenchmarkSerializeState(b *testing.B) {
	state := &domain.GameState{
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
			"p1": {ID: "p1", PlayerIndex: 0, Nickname: "TestPlayer", Palette: 1, ScoreContribution: 50},
			"p2": {ID: "p2", PlayerIndex: 1, Nickname: "AnotherPlayer", Palette: 2, ScoreContribution: 30},
		},
		NextPlayerIndex:     2,
		TickCount:           42,
		StartedAt:           1700000000,
		SessionID:           "sess-123",
		LobbyCode:           domain.RoomCode("ABCDE"),
		Wind:                0.3,
		WindTarget:          -0.2,
		WindChangeCountdown: 100,
		WindMicroCountdown:  5,
		WindMidCountdown:    50,
		WindMidOffset:       0.01,
		RestartVotes:        map[string]bool{"p1": true},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = SerializeState(state)
	}
}

func BenchmarkDeserializeState(b *testing.B) {
	state := &domain.GameState{
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
			"p1": {ID: "p1", PlayerIndex: 0, Nickname: "TestPlayer", Palette: 1, ScoreContribution: 50},
		},
		NextPlayerIndex:     1,
		TickCount:           42,
		StartedAt:           1700000000,
		SessionID:           "sess-123",
		LobbyCode:           domain.RoomCode("ABCDE"),
		Wind:                0.3,
		WindTarget:          -0.2,
		WindChangeCountdown: 100,
		WindMicroCountdown:  5,
		WindMidCountdown:    50,
		WindMidOffset:       0.01,
		RestartVotes:        map[string]bool{"p1": true},
	}
	data, _ := SerializeState(state)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = DeserializeState(data)
	}
}

func BenchmarkNewGameState(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewGameState("BENCH", 42, testRNG())
	}
}

// ─── 辅助函数 ────────────────────────────────────────────────────────
