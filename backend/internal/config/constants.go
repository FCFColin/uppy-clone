// Package config holds application-wide configuration constants.
package config

import "time"

// AccessTokenTTL is the lifetime of access tokens.
const AccessTokenTTL = 15 * time.Minute

// RefreshTokenTTL is the lifetime of refresh tokens.
const RefreshTokenTTL = 7 * 24 * time.Hour

// MagicLinkTTL is the lifetime of magic link tokens.
const MagicLinkTTL = 10 * time.Minute

// AdminTokenTTL is the lifetime of admin session tokens.
const AdminTokenTTL = 30 * time.Minute

// WSReadLimit is the maximum size in bytes of a single WebSocket message.
const WSReadLimit = 4096

// WSChannelBuffer is the buffer size for per-connection outbound channels.
const WSChannelBuffer = 64

// MessageWindowMs is the message rate-limit window length in milliseconds.
const MessageWindowMs = 60_000

// DefaultPageSize is the default number of items returned by paginated endpoints.
const DefaultPageSize = 50

// MaxPageSize is the maximum number of items a paginated endpoint will return.
const MaxPageSize = 100

// CookieMaxAge is the max-age (in seconds) for auth cookies.
const CookieMaxAge = 900

// MaxWSConnections is the global cap on concurrent WebSocket connections.
const MaxWSConnections = 1000

// MaxPlayersPerRoom limits how many players can join a single room (misc-026).
// It is lower than protocol.MaxPlayers (100) to ensure playability and performance.
const MaxPlayersPerRoom = 50

// MaxFailedLogins is the number of failed attempts before a login lockout.
const MaxFailedLogins = 5

// LoginLockoutTTL is how long an account stays locked after too many failures.
const LoginLockoutTTL = 15 * time.Minute

// RoomCodeLen is the length of generated room codes.
const RoomCodeLen = 5

// MagicLinkTokenLen is the length of generated magic link tokens.
const MagicLinkTokenLen = 64

// BcryptMaxLen is the maximum password length accepted by bcrypt.
const BcryptMaxLen = 72

// IdempotencyKeyMaxLen is the maximum length of an idempotency key.
const IdempotencyKeyMaxLen = 255

// AuthRateLimitMax is the maximum auth requests allowed per window.
const AuthRateLimitMax = 5

// AuthRateLimitWindow is the duration of the auth rate-limit window.
const AuthRateLimitWindow = 1 * time.Minute

// AdminRateLimitMax is the maximum admin requests allowed per window.
const AdminRateLimitMax = 10

// AdminRateLimitWindow is the duration of the admin rate-limit window.
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

// CleanupInterval is how often background cleanup tasks run.
const CleanupInterval = 60 * time.Second

// MetricsInterval is how often metrics are scraped or flushed.
const MetricsInterval = 15 * time.Second

// StaticCacheMaxAge is the max-age (seconds) for static asset caching.
const StaticCacheMaxAge = 86400

// JWTIssuer is the issuer claim set on JWTs (auth-002).
const JWTIssuer = "balloon-game"

// JWTAudience is the audience claim set on JWTs (auth-002).
const JWTAudience = "balloon-game-users"

// DefaultOTLPInsecure controls whether OTLP uses TLS by default (audit-011).
// Production deployments must set OTLP_INSECURE=true explicitly for dev.
const DefaultOTLPInsecure = false
