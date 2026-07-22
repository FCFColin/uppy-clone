package domain

import "math"

// GamePhase represents the current phase of a game.
type GamePhase string

// Game phase constants.
const (
	PhaseWaiting   GamePhase = "waiting"
	PhaseCountdown GamePhase = "countdown"
	PhasePlaying   GamePhase = "playing"
	PhaseEnded     GamePhase = "ended"
)

// BalloonState holds the state of the balloon game object.
type BalloonState struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	VX    float64 `json:"vx"`
	VY    float64 `json:"vy"`
	Score int     `json:"score"`
}

// Validate validates the balloon state fields.
func (b *BalloonState) Validate() error {
	// store-029: Check NaN and add Y upper bound. X and Y are normalized [0,1].
	// NaN comparisons always return false, so explicit math.IsNaN checks are needed.
	if math.IsNaN(b.X) || math.IsNaN(b.Y) || math.IsNaN(b.VX) || math.IsNaN(b.VY) {
		return ErrValidation
	}
	if b.Y < 0 || b.Y > 1 || b.X < 0 || b.X > 1 {
		return ErrValidation
	}
	return nil
}

// BirdState holds the state of the bird game object.
type BirdState struct {
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	VX         float64 `json:"vx"`
	VY         float64 `json:"vy"`
	Active     bool    `json:"active"`
	SpawnTimer int     `json:"spawnTimer"`
}

// Validate checks the bird state for out-of-range coordinates.
func (b *BirdState) Validate() error {
	if b.Y < -1 || b.Y > 2 || b.X < -1 || b.X > 2 {
		return ErrValidation
	}
	return nil
}

// GhostState holds the state of the ghost game object.
type GhostState struct {
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	VX         float64 `json:"vx"`
	VY         float64 `json:"vy"`
	Active     bool    `json:"active"`
	SpawnTimer int     `json:"spawnTimer"`
	RepelTimer int     `json:"repelTimer"`
}

// Validate checks the ghost state for out-of-range coordinates.
func (g *GhostState) Validate() error {
	if g.Y < -1 || g.Y > 2 || g.X < -1 || g.X > 2 {
		return ErrValidation
	}
	return nil
}

// PlayerState represents a player in a room.
type PlayerState struct {
	ID                 string `json:"id"`
	PlayerIndex        int    `json:"playerIndex"`
	Nickname           string `json:"nickname"`
	Palette            int    `json:"palette"`
	CooldownEndTime    int64  `json:"cooldownEndTime"`
	ScoreContribution  int    `json:"scoreContribution"`
	TapsCount          int    `json:"tapsCount"`
	MessageCount       int    `json:"messageCount"`
	MessageWindowStart int64  `json:"messageWindowStart"`
	LastNicknameChange int64  `json:"lastNicknameChange"`
	NicknameConfirmed  bool   `json:"nicknameConfirmed"`
	Disconnected       bool   `json:"disconnected"`
	DisconnectedAt     *int64 `json:"disconnectedAt"`
}

// CanTap checks whether the player's cooldown has elapsed.
func (p *PlayerState) CanTap(now int64) bool {
	return now >= p.CooldownEndTime
}

// RecordTap records a tap: sets the new cooldown end time and increments stats.
func (p *PlayerState) RecordTap(now int64, cooldown int64) {
	if p.ScoreContribution >= MaxScore {
		return
	}
	p.CooldownEndTime = now + cooldown
	p.TapsCount++
	p.ScoreContribution++
}

// MarkDisconnected 标记玩家为断连并记录断连时间戳（进入优雅期）。
func (p *PlayerState) MarkDisconnected(now int64) {
	p.Disconnected = true
	p.DisconnectedAt = &now
}

// GameState represents the complete game state for a room (aggregate).
type GameState struct {
	Phase               GamePhase               `json:"phase"`
	Balloon             BalloonState            `json:"balloon"`
	Bird                BirdState               `json:"bird"`
	Ghost               GhostState              `json:"ghost"`
	Players             map[string]*PlayerState `json:"players"`
	NextPlayerIndex     int                     `json:"nextPlayerIndex"`
	TickCount           int                     `json:"tickCount"`
	StartedAt           int64                   `json:"startedAt"`
	CreatedAt           int64                   `json:"createdAt"`
	SessionID           string                  `json:"sessionId"`
	LobbyCode           RoomCode                `json:"lobbyCode"`
	Wind                float64                 `json:"wind"`
	WindTarget          float64                 `json:"windTarget"`
	WindChangeCountdown int                     `json:"windChangeCountdown"`
	WindMicroCountdown  int                     `json:"windMicroCountdown"`
	WindMidCountdown    int                     `json:"windMidCountdown"`
	WindMidOffset       float64                 `json:"windMidOffset"`
	RestartVotes        map[string]bool         `json:"restartVotes"`
	RestartTimerStart   *int64                  `json:"restartTimerStart"`
	RNGSeed             int64                   `json:"rngSeed"`
}

// IsGameOver 检查游戏是否已结束。
func (g *GameState) IsGameOver() bool {
	return g.Phase == PhaseEnded
}

// NewGameState 创建一个新的 GameState 实例，初始化 Maps。
func NewGameState() *GameState {
	return &GameState{
		Players:      make(map[string]*PlayerState),
		RestartVotes: make(map[string]bool),
	}
}
