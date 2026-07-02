// Package protocol defines the binary protocol for game communication.
package protocol

import (
	"encoding/binary"

	"github.com/uppy-clone/backend/internal/constants"
)

// le is the package-level little-endian byte order for binary encoding.
var le = binary.LittleEndian

// ─── 客户端消息类型（浏览器 → 服务端） ──────────────────────────────

const (
	MsgTap         = constants.MsgTap
	MsgSetNickname = constants.MsgSetNickname
	MsgRestartVote = constants.MsgRestartVote
	MsgPing        = constants.MsgPing
)

// ─── 服务端消息类型（服务端 → 浏览器） ──────────────────────────────

const (
	MsgSnapshot        = constants.MsgSnapshot
	MsgPlayerJoin      = constants.MsgPlayerJoin
	MsgPlayerLeave     = constants.MsgPlayerLeave
	MsgTapAccepted     = constants.MsgTapAccepted
	MsgTapRejected     = constants.MsgTapRejected
	MsgGameStateChange = constants.MsgGameStateChange
	MsgRestartStatus   = constants.MsgRestartStatus
	MsgPong            = constants.MsgPong
)

// ─── 游戏阶段编码 ────────────────────────────────────────────────────

// Phase codes for binary protocol.
const (
	PhaseCodeWaiting   = 0
	PhaseCodePlaying   = 1
	PhaseCodeEnded     = 2
	PhaseCodeCountdown = 3
)

// End reason codes (sent as optional 3rd byte on MsgGameStateChange when phase=ended).
const (
	EndReasonNone   = 0
	EndReasonGround = 1
	EndReasonBird   = 2
	EndReasonGhost  = 3
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

// ─── 物理常量（由 go generate 自动同步到前端 constants.ts） ──────
//go:generate go run ../../cmd/gen-frontend-constants

// Physics constants.
const (
	// @ts PHYSICS.GRAVITY
	Gravity = 0.0005
	// @ts PHYSICS.TAP_FORCE
	TapForce = 0.025
	// @ts PHYSICS.TAP_RANGE
	TapRange = 0.35
	// @ts PHYSICS.TICK_RATE
	TickRate = 15 // Hz
	// @ts PHYSICS.TICK_INTERVAL
	TickInterval = 1000.0 / float64(TickRate) // ≈66.67ms
	// @ts PHYSICS.MAX_PLAYERS
	MaxPlayers = 100
	// @ts PHYSICS.COUNTDOWN_DURATION
	CountdownTicks = 45 // 3s x 15Hz
	// @ts PHYSICS.GAME_RESET_DELAY
	EndResetTicks = 450 // 30s x 15Hz
	// @ts PHYSICS.BIRD_SPAWN_MIN
	BirdSpawnMin = 15 // 1s x 15Hz
	// @ts PHYSICS.BIRD_SPAWN_MAX
	BirdSpawnMax = 30 // 2s x 15Hz
	// @ts PHYSICS.BIRD_SPEED
	BirdSpeed = 0.003
	// @ts PHYSICS.BIRD_COLLISION_RADIUS
	BirdCollisionRadius = 0.035

	// @ts PHYSICS.GHOST_SPAWN_MIN
	GhostSpawnMin = 15 // 1s x 15Hz
	// @ts PHYSICS.GHOST_SPAWN_MAX
	GhostSpawnMax = 30 // 2s x 15Hz
	// @ts PHYSICS.GHOST_SPEED
	GhostSpeed = 0.002
	// @ts PHYSICS.GHOST_ATTRACT_RADIUS
	GhostAttractRadius = 0.4
	// @ts PHYSICS.GHOST_COLLISION_RADIUS_X
	GhostCollisionRadiusX = 0.035
	// @ts PHYSICS.GHOST_COLLISION_RADIUS_Y
	GhostCollisionRadiusY = 0.045
	// @ts PHYSICS.GHOST_REPEL_RADIUS
	GhostRepelRadius = 0.15
	// @ts PHYSICS.GHOST_REPEL_DURATION
	GhostRepelDuration = 30 // 2s x 15Hz
	// @ts PHYSICS.GHOST_REPEL_FORCE
	GhostRepelForce = 0.01
	// @ts PHYSICS.GHOST_WANDER_CHANGE_INTERVAL
	GhostWanderChangeInterval = 30
	// @ts PHYSICS.GHOST_DAMAGE
	GhostDamage = 0.015

	// @ts PHYSICS.WIND_MAX
	WindMax = 0.0002
	// @ts PHYSICS.WIND_LERP_RATE
	WindLerpRate = 0.012
	// @ts PHYSICS.WIND_CHANGE_INTERVAL
	WindChangeInterval = 225
	// @ts PHYSICS.WIND_JITTER
	WindJitter = 0.000025
	// @ts PHYSICS.WIND_MICRO_INTERVAL
	WindMicroInterval = 10
	// @ts PHYSICS.WIND_MID_INTERVAL
	WindMidInterval = 75
	// @ts PHYSICS.WIND_MID_MAGNITUDE
	WindMidMagnitude = 0.0002
	// @ts PHYSICS.WIND_CLAMP
	WindClamp = 0.65
	// @ts PHYSICS.WIND_TARGET_SPAN
	WindTargetSpan = 1.0
	// @ts PHYSICS.WIND_EDGE_SOFT_ZONE
	WindEdgeSoftZone = 0.12

	// @ts PHYSICS.HORIZONTAL_DRAG
	HorizontalDrag = 0.98

	// @ts PHYSICS.INTERP_DELAY_MS
	InterpDelayMs = 100
)

// ─── 冷却公式参数（由 go generate 自动同步到前端） ────────────────

// Cooldown constants.
const (
	// @ts COOLDOWN.BASE_MS
	CooldownBaseMs = 1000
	// @ts COOLDOWN.LOG_COEFFICIENT
	CooldownLogCoeff = 2032
	// @ts COOLDOWN.MAX_MS
	CooldownMaxMs = 15000
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
