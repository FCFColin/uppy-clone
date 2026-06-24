// Package domain defines the core domain types for the multiplayer game.
package domain

// User represents a registered user.
type User struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Nickname  string `json:"nickname"`
	Palette   int    `json:"palette"`
	CreatedAt int64  `json:"created_at"`
	LastLogin *int64 `json:"last_login"`
}

// GameSession represents a game session record.
type GameSession struct {
	ID         string  `json:"id"`
	LobbyCode  string  `json:"lobby_code"`
	CreatedBy  *string `json:"created_by"`
	Status     string  `json:"status"`
	StartedAt  *int64  `json:"started_at"`
	EndedAt    *int64  `json:"ended_at"`
	FinalScore int     `json:"final_score"`
}

// GameResult represents a single player result in a game session.
type GameResult struct {
	ID                string `json:"id"`
	SessionID         string `json:"session_id"`
	UserID            string `json:"user_id"`
	ScoreContribution int    `json:"score_contribution"`
	TapsCount         int    `json:"taps_count"`
	CreatedAt         int64  `json:"created_at"`
}

// LobbyState stores serialized game state for a lobby room.
type LobbyState struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	State     string `json:"state"`
	UpdatedAt int64  `json:"updated_at"`
	CreatedAt int64  `json:"created_at"`
}

// AppConfig stores admin configuration as JSON.
type AppConfig struct {
	ID            string `json:"id"`
	Config        string `json:"config"`
	UpdatedAt     int64  `json:"updated_at"`
	EmailEnabled  bool   `json:"email_enabled"`
	ResendAPIKey  string `json:"resend_api_key"`
	EmailFrom     string `json:"email_from"`
	AdminPassword string `json:"admin_password"`
}

// --- Game state types (matching TypeScript version) ---

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

// BirdState holds the state of the bird game object.
type BirdState struct {
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	VX         float64 `json:"vx"`
	VY         float64 `json:"vy"`
	Active     bool    `json:"active"`
	SpawnTimer int     `json:"spawnTimer"`
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
	p.CooldownEndTime = now + cooldown
	p.TapsCount++
	p.ScoreContribution++
}

// IsRateLimited 检查玩家在当前消息窗口内是否已被速率限制。
// windowMs 为窗口长度（毫秒），maxMessages 为窗口内最大消息数。
func (p *PlayerState) IsRateLimited(now int64, windowMs int64, maxMessages int) bool {
	if now-p.MessageWindowStart > windowMs {
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
	LobbyCode           string                  `json:"lobbyCode"`
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
func (g *GameState) AddPlayer(p *PlayerState) {
	g.Players[p.ID] = p
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
