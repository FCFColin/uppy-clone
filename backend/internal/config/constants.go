// Package config defines application-wide configuration constants.
package config

import "time"

// AccessTokenTTL is the duration for which an access token is valid.
const AccessTokenTTL = 15 * time.Minute

// RefreshTokenTTL is the duration for which a refresh token is valid.
const RefreshTokenTTL = 7 * 24 * time.Hour

// MagicLinkTTL is the duration for which a magic link is valid.
const MagicLinkTTL = 15 * time.Minute

// AdminTokenTTL is the duration for which an admin token is valid.
const AdminTokenTTL = 30 * time.Minute

// WSReadLimit is the maximum message size for WebSocket reads.
const WSReadLimit = 4096

// WSChannelBuffer is the buffer size for WebSocket message channels.
const WSChannelBuffer = 64

// MessageWindowMs is the time window for message rate limiting in milliseconds.
const MessageWindowMs = 60_000

// DefaultPageSize is the default number of items per page.
const DefaultPageSize = 50

// MaxPageSize is the maximum number of items per page.
const MaxPageSize = 100

// CookieMaxAge is the max-age of cookies in seconds.
const CookieMaxAge = 900

// MaxWSConnections is the maximum number of WebSocket connections per IP.
const MaxWSConnections = 1000

// MaxPlayersPerRoom is the maximum number of players in a game room.
const MaxPlayersPerRoom = 50

// MaxFailedLogins is the maximum number of failed login attempts before lockout.
const MaxFailedLogins = 5

// LoginLockoutTTL is the duration of the login lockout period.
const LoginLockoutTTL = 15 * time.Minute

// MaxNicknameLen is the maximum length of a nickname.
const MaxNicknameLen = 12

// RoomCodeLen is the length of a room code.
const RoomCodeLen = 5

// MagicLinkTokenLen is the length of a magic link token (hex-encoded SHA-256 = 64 chars).
const MagicLinkTokenLen = 64

// BcryptMaxLen is the maximum password length for bcrypt (72 bytes).
const BcryptMaxLen = 72

// IdempotencyKeyMaxLen is the maximum length of an idempotency key.
const IdempotencyKeyMaxLen = 255

// AuthRateLimitMax is the maximum number of auth requests per window.
const AuthRateLimitMax = 5

// AuthRateLimitWindow is the time window for auth rate limiting.
const AuthRateLimitWindow = 1 * time.Minute

// AdminRateLimitMax is the maximum number of admin requests per window.
const AdminRateLimitMax = 10

// AdminRateLimitWindow is the time window for admin rate limiting.
const AdminRateLimitWindow = 5 * time.Minute

// DefaultPort is the default HTTP server port.
const DefaultPort = "8080"

// ServerReadTimeout is the HTTP server read timeout.
const ServerReadTimeout = 15 * time.Second

// ServerWriteTimeout is the HTTP server write timeout.
const ServerWriteTimeout = 15 * time.Second

// ServerIdleTimeout is the HTTP server idle timeout.
const ServerIdleTimeout = 60 * time.Second

// ShutdownTimeout is the maximum time to wait for graceful shutdown.
const ShutdownTimeout = 30 * time.Second

// CleanupInterval is the interval for background cleanup jobs.
const CleanupInterval = 60 * time.Second

// MetricsInterval is the interval for metrics collection.
const MetricsInterval = 15 * time.Second

// StaticCacheMaxAge is the max-age for static file cache in seconds (24h).
const StaticCacheMaxAge = 86400
