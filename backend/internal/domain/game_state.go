package domain

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

func (b *BalloonState) Validate() error {
	if b.Y < 0 {
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

func (b *BirdState) Validate() error {
	if b.Y < 0 {
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

func (g *GhostState) Validate() error {
	if g.Y < 0 {
		return ErrValidation
	}
	return nil
}

// PlayerState 表示房间内一个玩家的状态。
// P3-1.1：升级为充血对象，业务规则（冷却、速率限制、断连/重连）封装在方法内。
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

// CanTap 检查玩家是否可以点击（冷却已结束）。
// 企业为何需要：将冷却判断封装在领域对象内，防止外部代码绕过业务规则。
func (p *PlayerState) CanTap(now int64) bool {
	return now >= p.CooldownEndTime
}

// RecordTap 记录一次点击：设置新冷却结束时间并累加统计。
// 企业为何需要：点击统计与冷却更新是原子业务操作，封装避免遗漏字段。
func (p *PlayerState) RecordTap(now int64, cooldown int64) {
	if p.ScoreContribution >= MaxScore {
		return
	}
	p.CooldownEndTime = now + cooldown
	p.TapsCount++
	p.ScoreContribution++
}

// IsRateLimited 检查玩家在当前消息窗口内是否已被速率限制。
// windowMs 为窗口长度（毫秒），maxMessages 为窗口内最大消息数。
func (p *PlayerState) IsRateLimited(now int64, windowMs int64, maxMessages int) bool {
	if now-p.MessageWindowStart > windowMs {
		p.MessageCount = 0
		p.MessageWindowStart = now
		return false
	}
	return p.MessageCount > maxMessages
}

// MarkDisconnected 标记玩家为断连并记录断连时间戳（进入优雅期）。
func (p *PlayerState) MarkDisconnected(now int64) {
	p.Disconnected = true
	p.DisconnectedAt = &now
}

// Reconnect 清除断连状态（重连时调用）。
func (p *PlayerState) Reconnect() {
	p.Disconnected = false
	p.DisconnectedAt = nil
}

// GameState 表示一个房间的完整游戏状态（聚合）。
// P3-1.2：添加 AddPlayer/RemovePlayer/IsGameOver 聚合方法。
type GameState struct {
	Phase               GamePhase               `json:"phase"`
	Balloon             BalloonState            `json:"balloon"`
	Bird                BirdState               `json:"bird"`
	Ghost               GhostState              `json:"ghost"`
	Players             map[string]*PlayerState `json:"players"`
	NextPlayerIndex     int                     `json:"nextPlayerIndex"`
	TickCount           int                     `json:"tickCount"`
	StartedAt           int64                   `json:"startedAt"`
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
}

// AddPlayer 添加一个玩家到游戏状态。
// 企业为何需要：聚合方法统一玩家加入入口，便于未来加入不变量校验。
func (g *GameState) AddPlayer(p *PlayerState) error {
	if g.Players == nil {
		g.Players = make(map[string]*PlayerState)
	}
	if _, exists := g.Players[p.ID]; exists {
		return ErrDuplicateUser
	}
	g.Players[p.ID] = p
	return nil
}

// RemovePlayer 从游戏状态移除指定玩家。
func (g *GameState) RemovePlayer(userID string) {
	delete(g.Players, userID)
}

// UpdatePlayerState 对指定玩家应用更新函数。
// 企业为何需要：聚合方法控制玩家状态变更入口，便于审计与不变量校验。
func (g *GameState) UpdatePlayerState(userID string, fn func(p *PlayerState)) {
	if p, ok := g.Players[userID]; ok {
		fn(p)
	}
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
