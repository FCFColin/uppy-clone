package game

import (
	"context"
	"encoding/json"
	"math"
	"math/rand/v2"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/nicknames"
	"github.com/uppy-clone/backend/internal/protocol"
	"github.com/uppy-clone/backend/internal/validate"
)

// ─── RNG ──────────────────────────────────────────────────────────────

// RNGSource is the interface for random number generation used by the game.
type RNGSource interface {
	Float64() float64
	IntN(n int) int
}

type seededRNG struct {
	rng *rand.Rand
}

func (s *seededRNG) Float64() float64 {
	return s.rng.Float64()
}

func (s *seededRNG) IntN(n int) int {
	return s.rng.IntN(n)
}

func newSeededRNG(seed int64) *seededRNG {
	return &seededRNG{rng: rand.New(rand.NewPCG(uint64(seed), uint64(seed^0xDEADBEEF)))} //nolint:gosec // G404: game RNG, not crypto
}

// ─── Repository Interfaces ─────────────────────────────────────────────

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
}

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

// ─── Names & Nicknames ───────────────────────────────────────────────

const roomAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// GenerateRoomCode 生成 config.RoomCodeLen 字符房间码
func GenerateRoomCode(rng RNGSource) string {
	code := make([]byte, config.RoomCodeLen)
	for i := range code {
		code[i] = roomAlphabet[rng.IntN(len(roomAlphabet))]
	}
	return string(code)
}

const maxNicknameLength = 12

// GenerateRandomNickname 从名字池随机组合生成昵称
func GenerateRandomNickname(usedNames map[string]bool) string {
	return nicknames.GenerateRandom(usedNames)
}

// GenerateUniqueNickname 生成不重复的随机昵称
func GenerateUniqueNickname(clientName string, usedNames map[string]bool) string {
	if clientName != "" {
		if validate.NicknameInputRejected(clientName) {
			return GenerateRandomNickname(usedNames)
		}
		truncated := clientName
		runeSlice := []rune(truncated)
		if len(runeSlice) > maxNicknameLength {
			truncated = string(runeSlice[:maxNicknameLength])
		}
		if truncated != "" && !usedNames[truncated] {
			return truncated
		}
	}
	return GenerateRandomNickname(usedNames)
}

// SanitizePlayerName 清理玩家名字：去除 XSS 向量、限制长度、折叠空白
func SanitizePlayerName(raw string) string {
	return validate.Nickname(raw)
}

// HandleSetNickname 处理设置昵称请求
//
// 包含 30 秒冷却（首次改名跳过冷却），防止频繁修改。
// 验证：长度字段越界检查、控制字符和 HTML 特殊字符过滤、空昵称忽略、
// 长度限制 12 字符、当前房间内重复检查。
func HandleSetNickname(_ *domain.GameState, player *domain.PlayerState, nickname string, usedNames map[string]bool) bool {
	now := time.Now().UnixMilli()

	// 首次改名（lastNicknameChange === 0）跳过冷却
	if player.LastNicknameChange != 0 && now-player.LastNicknameChange < domain.NicknameCooldownMs {
		return false
	}

	// 内容过滤与长度限制（domain.Nickname 委托 validate.Nickname）
	parsed, err := domain.NewNickname(nickname, validate.DefaultValidator)
	if err != nil {
		return false
	}
	nickname = parsed.String()

	// 与当前昵称相同则无需修改
	if nickname == player.Nickname {
		return false
	}

	// 重复检查：若与 usedNames 重复，服务端重新生成不重复的名字
	if usedNames[nickname] {
		nickname = GenerateUniqueNickname(nickname, usedNames)
	}

	// 更新 usedNames：移除旧名、加入新名
	delete(usedNames, player.Nickname)
	usedNames[nickname] = true

	player.LastNicknameChange = now
	player.Nickname = nickname
	return true
}

// ─── Game State ──────────────────────────────────────────────────────

// NewGameState creates a default game state.
func NewGameState(lobbyCode string, seed int64, rng RNGSource) *domain.GameState {
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
		RNGSeed:           seed,
	}
	initWind(state, rng)
	return state
}

// ResetGameEntities resets game entities (balloon, bird, ghost) while preserving the player list.
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

// SerializeState marshals the game state to JSON bytes.
func SerializeState(state *domain.GameState) ([]byte, error) {
	return json.Marshal(state)
}

// DeserializeState unmarshals JSON bytes into a game state.
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

// initialWind 生成随机初始风速和风向。
// 游戏一开始就应有风场，让玩家立即感受到环境互动。
func initialWind(rng RNGSource) (wind, windTarget float64) {
	windTarget = (rng.Float64() - 0.5) * protocol.WindTargetSpan
	wind = max(-protocol.WindClamp, min(protocol.WindClamp, windTarget*0.6))
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
