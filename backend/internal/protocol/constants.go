// Package protocol defines the binary protocol for game communication.
package protocol

import "encoding/binary"

// le is the package-level little-endian byte order for binary encoding.
var le = binary.LittleEndian

// ─── 客户端消息类型（浏览器 → 服务端） ──────────────────────────────

// Message type constants.
const (
	MsgTap         = 0x10
	MsgSetNickname = 0x11
	MsgRestartVote = 0x12
	MsgPing        = 0x20
)

// ─── 服务端消息类型（服务端 → 浏览器） ──────────────────────────────

// Message type constants.
const (
	MsgSnapshot        = 0x01
	MsgPlayerJoin      = 0x02
	MsgPlayerLeave     = 0x03
	MsgTapAccepted     = 0x04
	MsgTapRejected     = 0x05
	MsgGameStateChange = 0x06
	MsgRestartStatus   = 0x07
	MsgPong            = 0x21
)

// ─── 游戏阶段编码 ────────────────────────────────────────────────────

// Phase codes for binary protocol.
const (
	PhaseCodeWaiting   = 0
	PhaseCodePlaying   = 1
	PhaseCodeEnded     = 2
	PhaseCodeCountdown = 3
)

// ─── 游戏阶段类型 ────────────────────────────────────────────────────

// GamePhase represents the current phase of a game.
type GamePhase string

// Game phase string constants.
const (
	PhaseWaiting   GamePhase = "waiting"
	PhaseCountdown GamePhase = "countdown"
	PhasePlaying   GamePhase = "playing"
	PhaseEnded     GamePhase = "ended"
)

// PhaseToCode 将游戏阶段转换为二进制编码值
func PhaseToCode(phase GamePhase) uint8 {
	switch phase {
	case PhaseWaiting:
		return PhaseCodeWaiting
	case PhasePlaying:
		return PhaseCodePlaying
	case PhaseEnded:
		return PhaseCodeEnded
	case PhaseCountdown:
		return PhaseCodeCountdown
	default:
		return PhaseCodeWaiting
	}
}

// CodeToPhase 将二进制编码值转换为游戏阶段
func CodeToPhase(code uint8) GamePhase {
	switch code {
	case PhaseCodeWaiting:
		return PhaseWaiting
	case PhaseCodePlaying:
		return PhasePlaying
	case PhaseCodeEnded:
		return PhaseEnded
	case PhaseCodeCountdown:
		return PhaseCountdown
	default:
		return PhaseWaiting
	}
}

// ─── 协议数据结构 ────────────────────────────────────────────────────

// BalloonState 气球状态（仅包含二进制协议传输的字段）
type BalloonState struct {
	X  float32
	Y  float32
	Vy float32
	Vx float32
}

// BirdState 鸟状态（仅包含二进制协议传输的字段）
type BirdState struct {
	X      float32
	Y      float32
	Active bool
}

// GhostState 幽灵状态（仅包含二进制协议传输的字段）
type GhostState struct {
	X          float32
	Y          float32
	Active     bool
	RepelTimer uint16
}

// PlayerState 玩家状态（仅包含二进制协议传输的字段）
type PlayerState struct {
	PlayerIndex       uint16
	CooldownMs        uint32 // 剩余冷却毫秒数
	Palette           uint32
	ScoreContribution uint32
	Nickname          string
}

// Ripple 涟漪效果（仅包含二进制协议传输的字段）
type Ripple struct {
	PlayerIndex uint16
	X           float32
	Y           float32
}

// ─── 物理常量（必须与 src/types.ts PHYSICS 完全一致） ──────────────

// Physics constants.
const (
	Gravity        = 0.0005
	TapForce       = 0.025
	TapRange       = 0.35
	TickRate       = 15 // Hz
	TickInterval   = 1000.0 / float64(TickRate)
	MaxPlayers     = 100
	CountdownTicks = 45  // 3s x 15Hz
	EndResetTicks  = 450 // 30s x 15Hz

	BirdSpawnMin        = 300 // 20s x 15Hz
	BirdSpawnMax        = 600 // 40s x 15Hz
	BirdSpeed           = 0.003
	BirdCollisionRadius = 0.05

	GhostSpawnMin             = 15 // 1s x 15Hz
	GhostSpawnMax             = 30 // 2s x 15Hz
	GhostSpeed                = 0.002
	GhostAttractRadius        = 0.4
	GhostCollisionRadius      = 0.06
	GhostRepelRadius          = 0.15
	GhostRepelDuration        = 30 // 2s x 15Hz
	GhostRepelForce           = 0.01
	GhostWanderChangeInterval = 30
	GhostDamage               = 0.015

	WindMax            = 0.0006
	WindLerpRate       = 0.012
	WindChangeInterval = 225
	WindJitter         = 0.00005
	WindMicroInterval  = 10
	WindMidInterval    = 75
	WindMidMagnitude   = 0.0006

	HorizontalDrag = 0.98
)

// ─── 冷却公式参数（必须与 src/types.ts COOLDOWN 完全一致） ──────────

// Cooldown constants.
const (
	CooldownBaseMs   = 1000
	CooldownLogCoeff = 2032
	CooldownMaxMs    = 15000
)

// ─── 游戏逻辑常量 ────────────────────────────────────────────────────

// Score constants.
const (
	ScoreToWin         = 100
	MaxScore           = 9999
	ReconnectGraceMs   = 30000
	RestartTimeoutMs   = 30000
	AutoRestartMs      = 60000
	MaxNicknameLen     = 12
	NicknameCooldownMs = 30000
	MessageRateLimit   = 100 // per minute
)

// ─── 调色板颜色（10 色，与客户端 PALETTE_COLORS 对应） ──────────────

// PaletteColors defines the color palette for players.
var PaletteColors = [10][3]uint8{
	{233, 69, 96},   // #e94560 red
	{15, 52, 96},    // #0f3460 navy
	{83, 52, 131},   // #533483 purple
	{0, 180, 216},   // #00b4d8 cyan
	{6, 214, 160},   // #06d6a0 green
	{255, 209, 102}, // #ffd166 yellow
	{239, 71, 111},  // #ef476f pink
	{17, 138, 178},  // #118ab2 teal
	{7, 59, 76},     // #073b4c dark
	{247, 140, 107}, // #f78c6b orange
}
