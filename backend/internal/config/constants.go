package config

import "time"

const AccessTokenTTL = 15 * time.Minute
const RefreshTokenTTL = 7 * 24 * time.Hour
const MagicLinkTTL = 10 * time.Minute
const AdminTokenTTL = 30 * time.Minute
const WSReadLimit = 4096
const WSChannelBuffer = 64
const MessageWindowMs = 60_000
const DefaultPageSize = 50
const MaxPageSize = 100
const CookieMaxAge = 900
const MaxWSConnections = 1000
const MaxPlayersPerRoom = 50
const MaxFailedLogins = 5
const LoginLockoutTTL = 15 * time.Minute
const RoomCodeLen = 5
const MagicLinkTokenLen = 64
const BcryptMaxLen = 72
const IdempotencyKeyMaxLen = 255
const AuthRateLimitMax = 5
const AuthRateLimitWindow = 1 * time.Minute
const AdminRateLimitMax = 10
const AdminRateLimitWindow = 5 * time.Minute
const DefaultPort = "8080"
const ServerReadTimeout = 15 * time.Second
const ServerWriteTimeout = 15 * time.Second
const ServerIdleTimeout = 60 * time.Second
const ShutdownTimeout = 30 * time.Second
const CleanupInterval = 60 * time.Second
const MetricsInterval = 15 * time.Second
const StaticCacheMaxAge = 86400

// Production deployments must set OTLP_INSECURE=false in the environment.
const DefaultOTLPInsecure = true
