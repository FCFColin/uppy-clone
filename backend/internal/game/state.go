package game

import (
	"encoding/json"
	"math"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

// NewGameState 创建默认游戏状态（对应 TS createDefaultGameState）
func NewGameState(lobbyCode string, rng RNGSource) *domain.GameState {
	spawnTimer := RandomSpawnTimer(rng)
	ghost := spawnGhost(rng)

	state := &domain.GameState{
		Phase:             domain.PhaseWaiting,
		Balloon:           createInitialBalloon(),
		Bird:              createInitialBird(spawnTimer),
		Ghost:             ghost,
		Players:           make(map[string]*domain.PlayerState),
		NextPlayerIndex:   0,
		TickCount:         0,
		StartedAt:         0,
		SessionID:         "",
		LobbyCode:         domain.RoomCode(lobbyCode),
		RestartVotes:      make(map[string]bool),
		RestartTimerStart: nil,
	}
	initWind(state, rng)
	return state
}

// ResetGameEntities 重置游戏实体（气球、鸟、幽灵），保留玩家列表
func ResetGameEntities(state *domain.GameState, spawnTimer int, rng RNGSource) {
	state.Balloon = createInitialBalloon()
	state.Bird = createInitialBird(spawnTimer)
	state.Ghost = spawnGhost(rng)
	state.TickCount = 0

	initWind(state, rng)

	// 重置重启投票
	state.RestartVotes = make(map[string]bool)
	state.RestartTimerStart = nil
}

func SerializeState(state *domain.GameState) ([]byte, error) {
	return json.Marshal(state)
}

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

const (
	initWindChangeCountdown = 112
	initWindMicroCountdown  = 10
	initWindMidCountdown    = 75
)

// initWind 初始化风场状态（随机风速和风向 + 倒计时）
func initWind(state *domain.GameState, rng RNGSource) {
	wind, windTarget := initialWind(rng)
	state.Wind = wind
	state.WindTarget = windTarget
	state.WindChangeCountdown = initWindChangeCountdown
	state.WindMicroCountdown = initWindMicroCountdown
	state.WindMidCountdown = initWindMidCountdown
	state.WindMidOffset = 0
}

// --- 内部辅助函数 ---

// initialWind 生成随机初始风速和风向。
// 游戏一开始就应有风场，让玩家立即感受到环境互动。
func initialWind(rng RNGSource) (wind, windTarget float64) {
	windTarget = (rng.Float64() - 0.5) * protocol.WindTargetSpan
	// 初始风速设为目标值的 60%，使风场立即可感知但不至于过强
	wind = windTarget * 0.6
	if wind > protocol.WindClamp {
		wind = protocol.WindClamp
	}
	if wind < -protocol.WindClamp {
		wind = -protocol.WindClamp
	}
	return wind, windTarget
}

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
func spawnGhost(rng RNGSource) domain.GhostState {
	// 在可见区域内随机生成（避开边缘 15% 和气球起始位置附近）
	// x: 0.15-0.85，y: 0.3-0.75
	x := 0.15 + rng.Float64()*0.7
	y := 0.3 + rng.Float64()*0.45

	// 初始速度：随机方向漫步
	angle := rng.Float64() * 2 * math.Pi
	vx := math.Cos(angle) * protocol.GhostSpeed
	vy := math.Sin(angle) * protocol.GhostSpeed

	return domain.GhostState{
		X:          x,
		Y:          y,
		VX:         vx,
		VY:         vy,
		Active:     true,
		SpawnTimer: int(protocol.GhostSpawnMin + rng.Float64()*float64(protocol.GhostSpawnMax-protocol.GhostSpawnMin)),
		RepelTimer: 0,
	}
}
