package game

import (
	"encoding/json"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

// NewGameState 创建默认游戏状态（对应 TS createDefaultGameState）
func NewGameState(lobbyCode string) *domain.GameState {
	spawnTimer := RandomSpawnTimer()
	ghost := spawnGhost()

	return &domain.GameState{
		Phase:            domain.PhaseWaiting,
		Balloon:          createInitialBalloon(),
		Bird:             createInitialBird(spawnTimer),
		Ghost:            ghost,
		Players:          make(map[string]*domain.PlayerState),
		NextPlayerIndex:  0,
		TickCount:        0,
		StartedAt:        0,
		SessionID:        "",
		LobbyCode:        lobbyCode,
		Wind:             0,
		WindTarget:       0,
		WindChangeCountdown: 112, // 225 的 50%，首次大变化在 7.5s 左右
		WindMicroCountdown:  10,
		WindMidCountdown:    75,
		WindMidOffset:       0,
		RestartVotes:        make(map[string]bool),
		RestartTimerStart:   nil,
	}
}

// ResetGameEntities 重置游戏实体（气球、鸟、幽灵），保留玩家列表
func ResetGameEntities(state *domain.GameState, spawnTimer int) {
	state.Balloon = createInitialBalloon()
	state.Bird = createInitialBird(spawnTimer)
	state.Ghost = spawnGhost()
	state.TickCount = 0

	// 重置风场
	state.Wind = 0
	state.WindTarget = 0
	state.WindChangeCountdown = 112
	state.WindMicroCountdown = 10
	state.WindMidCountdown = 75
	state.WindMidOffset = 0

	// 重置重启投票
	state.RestartVotes = make(map[string]bool)
	state.RestartTimerStart = nil
}

// SerializeState 将 GameState 序列化为 JSON 字节
func SerializeState(state *domain.GameState) ([]byte, error) {
	return json.Marshal(state)
}

// DeserializeState 从 JSON 字节反序列化为 GameState
func DeserializeState(data []byte) (*domain.GameState, error) {
	var state domain.GameState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	// 确保 map 不为 nil
	if state.Players == nil {
		state.Players = make(map[string]*domain.PlayerState)
	}
	if state.RestartVotes == nil {
		state.RestartVotes = make(map[string]bool)
	}

	return &state, nil
}

// --- 内部辅助函数 ---

// createInitialBalloon 创建初始气球状态
func createInitialBalloon() domain.BalloonState {
	return domain.BalloonState{
		X:     0.5,
		Y:     0.95,
		VX:    0,
		VY:    0,
		Score: 0,
	}
}

// createInitialBird 创建初始鸟状态
func createInitialBird(spawnTimer int) domain.BirdState {
	return domain.BirdState{
		X:          0,
		Y:          0,
		VX:         0,
		VY:         0,
		Active:     false,
		SpawnTimer: spawnTimer,
	}
}

// spawnGhost 生成新幽灵（对应 TS spawnGhost(1,1)）
func spawnGhost() domain.GhostState {
	// 在可见区域内随机生成（避开边缘 15% 和气球起始位置附近）
	// x: 0.15-0.85，y: 0.3-0.75
	x := 0.15 + randFloat64()*0.7
	y := 0.3 + randFloat64()*0.45

	// 初始速度：随机方向漫步
	angle := randFloat64() * 2 * pi
	vx := cos(angle) * protocol.GhostSpeed
	vy := sin(angle) * protocol.GhostSpeed

	return domain.GhostState{
		X:          x,
		Y:          y,
		VX:         vx,
		VY:         vy,
		Active:     true,
		SpawnTimer: int(protocol.GhostSpawnMin + randFloat64()*float64(protocol.GhostSpawnMax-protocol.GhostSpawnMin)),
		RepelTimer: 0,
	}
}
