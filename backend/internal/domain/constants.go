package domain

import "errors"

const (
	// MaxScore is the hard cap on any player's score contribution.
	MaxScore = 9999
	// ReconnectGraceMs is the grace period (ms) before a disconnected player is removed.
	ReconnectGraceMs = 30000
	// RestartTimeoutMs is how long (ms) to wait for restart votes before aborting.
	RestartTimeoutMs = 30000
	// AutoRestartMs is the delay (ms) before a room auto-restarts after ending.
	AutoRestartMs = 60000
	// MaxNicknameLen is the maximum length of a player nickname.
	MaxNicknameLen = 12
	// NicknameCooldownMs is the minimum interval (ms) between nickname changes.
	NicknameCooldownMs = 30000
	// MessageRateLimit is the maximum messages per player within the rate-limit window.
	MessageRateLimit = 100
)

// ErrDuplicateUser is returned when a user already exists in a game or resource.
var ErrDuplicateUser = errors.New("duplicate user")

// ErrNotFound is returned when a requested resource does not exist.
var ErrNotFound = errors.New("resource not found")

// ErrValidation is returned when input fails validation checks.
var ErrValidation = errors.New("validation failed")
