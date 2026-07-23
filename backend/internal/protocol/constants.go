// Package protocol defines the binary protocol for game communication.
package protocol

import (
	"encoding/binary"
	"log/slog"
)

var le = binary.LittleEndian

const (
	// @ts CLIENT_MSG.TAP
	MsgTap = 0x10
	// @ts CLIENT_MSG.SET_NICKNAME
	MsgSetNickname = 0x11
	// @ts CLIENT_MSG.RESTART_VOTE
	MsgRestartVote = 0x12
	// @ts CLIENT_MSG.PING
	MsgPing = 0x20
)

const (
	// @ts MSG_TYPE.SNAPSHOT
	MsgSnapshot = 0x01
	// @ts MSG_TYPE.PLAYER_JOIN
	MsgPlayerJoin = 0x02
	// @ts MSG_TYPE.PLAYER_LEAVE
	MsgPlayerLeave = 0x03
	// @ts MSG_TYPE.TAP_ACCEPTED
	MsgTapAccepted = 0x04
	// @ts MSG_TYPE.TAP_REJECTED
	MsgTapRejected = 0x05
	// @ts MSG_TYPE.GAME_STATE_CHANGE
	MsgGameStateChange = 0x06
	// @ts MSG_TYPE.RESTART_STATUS
	MsgRestartStatus = 0x07
	// MsgNicknameRejected: 2nd byte carries a NickReject* reason code.
	// @ts MSG_TYPE.NICKNAME_REJECTED
	MsgNicknameRejected = 0x08
	// @ts MSG_TYPE.PONG
	MsgPong = 0x21
)

// Phase codes for binary protocol.
const (
	// @ts PHASE_CODE.WAITING
	PhaseCodeWaiting = 0
	// @ts PHASE_CODE.PLAYING
	PhaseCodePlaying = 1
	// @ts PHASE_CODE.ENDED
	PhaseCodeEnded = 2
	// @ts PHASE_CODE.COUNTDOWN
	PhaseCodeCountdown = 3
)

// End reason codes (sent as optional 3rd byte on MsgGameStateChange when phase=ended).
const (
	// @ts END_REASON.NONE
	EndReasonNone = 0
	// @ts END_REASON.GROUND
	EndReasonGround = 1
	// @ts END_REASON.BIRD
	EndReasonBird = 2
	// @ts END_REASON.GHOST
	EndReasonGhost = 3
)

// Nickname reject reason codes (sent as 2nd byte on MsgNicknameRejected).
const (
	// @ts NICKNAME_REJECT_REASON.EMPTY
	NickRejectEmpty = 0x01
	// @ts NICKNAME_REJECT_REASON.DUPLICATE
	NickRejectDuplicate = 0x02
	// @ts NICKNAME_REJECT_REASON.COOLDOWN
	NickRejectCooldown = 0x03
	// @ts NICKNAME_REJECT_REASON.DECODE_ERROR
	NickRejectDecodeError = 0x04
)

type GamePhase string

const (
	PhaseWaiting   GamePhase = "waiting"
	PhaseCountdown GamePhase = "countdown"
	PhasePlaying   GamePhase = "playing"
	PhaseEnded     GamePhase = "ended"
)

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
		slog.Warn("PhaseToCode: unknown phase", "phase", phase)
		return PhaseCodeWaiting
	}
}

type BalloonState struct {
	X  float32
	Y  float32
	Vy float32
	Vx float32
}

type BirdState struct {
	X      float32
	Y      float32
	Active bool
}

type GhostState struct {
	X          float32
	Y          float32
	Active     bool
	RepelTimer uint16
}

type PlayerState struct {
	PlayerIndex       uint16
	CooldownMs        uint32
	Palette           uint32
	ScoreContribution uint32
	Nickname          string
}

type Ripple struct {
	PlayerIndex uint16
	X           float32
	Y           float32
}

//go:generate go run ../../cmd/gen-frontend-constants

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
	// @ts PHYSICS.BIRD_COLLISION_RADIUS_X
	BirdCollisionRadiusX = 0.020
	// @ts PHYSICS.BIRD_COLLISION_RADIUS_Y
	BirdCollisionRadiusY = 0.035

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
	// @ts PHYSICS.BALLOON_COLLISION_RADIUS
	BalloonCollisionRadius = 0.06
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
	HorizontalDrag = 0.99

	// @ts PHYSICS.INTERP_DELAY_MS
	InterpDelayMs = 100
)

const (
	// @ts COOLDOWN.BASE_MS
	CooldownBaseMs = 1000
	// @ts COOLDOWN.LOG_COEFFICIENT
	CooldownLogCoeff = 2032
	// @ts COOLDOWN.MAX_MS
	CooldownMaxMs = 15000
)

// @ts PALETTE_COLORS
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
