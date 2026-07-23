package game

import (
	"testing"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

func TestNewGameState_InitialValues(t *testing.T) {
	state := NewGameState("TEST", 42, testRNG())

	if state.Phase != domain.PhaseWaiting {
		t.Fatalf("初始 phase 应为 waiting，got=%v", state.Phase)
	}
	if state.Balloon.X != 0.5 || state.Balloon.Y != 0.95 {
		t.Fatalf("初始气球位置应为 (0.5, 0.95)，got X=%v Y=%v", state.Balloon.X, state.Balloon.Y)
	}
	if !state.Ghost.Active {
		t.Fatal("初始幽灵应已激活")
	}
	if state.Wind == 0 {
		t.Fatalf("初始风场不应为 0（游戏开始即应有风），got=%v", state.Wind)
	}
	if state.WindChangeCountdown != 112 {
		t.Fatalf("初始 WindChangeCountdown 应为 112，got=%d", state.WindChangeCountdown)
	}
	if string(state.LobbyCode) != "TEST" {
		t.Fatalf("LobbyCode 应为 TEST，got=%v", string(state.LobbyCode))
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

func TestResetGameEntities_TableDriven(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(s *domain.GameState)
		assert func(t *testing.T, s *domain.GameState)
	}{
		{
			name: "ResetsBalloon",
			setup: func(s *domain.GameState) {
				s.Balloon.X = 0.3
				s.Balloon.Y = 0.5
				s.Balloon.VX = 0.1
				s.Balloon.VY = 0.2
				s.Balloon.Score = 500
				s.TickCount = 200
			},
			assert: func(t *testing.T, s *domain.GameState) {
				if s.Balloon.X != 0.5 {
					t.Fatalf("重置后气球 X 应为 0.5，got=%v", s.Balloon.X)
				}
				if s.Balloon.Y != 0.95 {
					t.Fatalf("重置后气球 Y 应为 0.95，got=%v", s.Balloon.Y)
				}
				if s.Balloon.VX != 0 || s.Balloon.VY != 0 {
					t.Fatalf("重置后气球速度应为 0，got VX=%v VY=%v", s.Balloon.VX, s.Balloon.VY)
				}
				if s.Balloon.Score != 0 {
					t.Fatalf("重置后分数应为 0，got=%d", s.Balloon.Score)
				}
				if s.TickCount != 0 {
					t.Fatalf("重置后 tickCount 应为 0，got=%d", s.TickCount)
				}
			},
		},
		{
			name: "ResetsGhost",
			setup: func(s *domain.GameState) {
				s.Ghost.Active = false
				s.Ghost.SpawnTimer = 100
			},
			assert: func(t *testing.T, s *domain.GameState) {
				if !s.Ghost.Active {
					t.Fatal("重置后幽灵应已激活")
				}
			},
		},
		{
			name: "ResetsWind",
			setup: func(s *domain.GameState) {
				s.Wind = 0.8
				s.WindTarget = -0.5
				s.WindChangeCountdown = 10
			},
			assert: func(t *testing.T, s *domain.GameState) {
				if s.Wind == 0 {
					t.Fatalf("重置后风场不应为 0（游戏开始即应有风），got=%v", s.Wind)
				}
				if s.WindTarget == 0 {
					t.Fatalf("重置后 WindTarget 不应为 0，got=%v", s.WindTarget)
				}
				if s.WindChangeCountdown != 112 {
					t.Fatalf("重置后 WindChangeCountdown 应为 112，got=%d", s.WindChangeCountdown)
				}
			},
		},
		{
			name: "ResetsVotes",
			setup: func(s *domain.GameState) {
				s.RestartVotes["player1"] = true
				s.RestartVotes["player2"] = true
				now := int64(1234567890)
				s.RestartTimerStart = &now
			},
			assert: func(t *testing.T, s *domain.GameState) {
				if len(s.RestartVotes) != 0 {
					t.Fatalf("重置后 RestartVotes 应为空，got=%d", len(s.RestartVotes))
				}
				if s.RestartTimerStart != nil {
					t.Fatal("重置后 RestartTimerStart 应为 nil")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := createTestState()
			tt.setup(state)
			ResetGameEntities(state, RandomSpawnTimer(testRNG()), testRNG())
			tt.assert(t, state)
		})
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

