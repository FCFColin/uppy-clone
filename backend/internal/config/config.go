// Package config holds application-wide configuration constants.
package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

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

// MaxPlayersPerRoom limits how many players can join a single room (misc-026).
// It is lower than protocol.MaxPlayers (100) to ensure playability and performance.
const MaxPlayersPerRoom = 50

const RoomCodeLen = 5
const MagicLinkTokenLen = 64
const BcryptMaxLen = 72
const DefaultPort = "8080"
const ServerReadTimeout = 15 * time.Second
const ServerWriteTimeout = 15 * time.Second
const ServerIdleTimeout = 60 * time.Second
const ShutdownTimeout = 30 * time.Second
const CleanupInterval = 60 * time.Second
const MetricsInterval = 15 * time.Second
const StaticCacheMaxAge = 86400

// JWTIssuer is the issuer claim set on JWTs (auth-002).
const JWTIssuer = "balloon-game"

// JWTAudience is the audience claim set on JWTs (auth-002).
const JWTAudience = "balloon-game-users"

const EnvProduction = "production"

type Env struct {
	JWTPrivateKey      string
	JWTPublicKey       string
	AdminJWTPrivateKey string
	AdminJWTPublicKey  string
	DatabaseURL        string
	RedisURL           string
	RedisEphemeralURL  string
	RedisRegionURL     string
	RedisPubSubURL     string
	EncryptionKey      string
	ResendAPIKey       string
	EmailFrom          string
	AdminPassword      string
	AuditSecret        string
	TrustedProxyCIDRs  string
	AllowedOrigins     string
	Port               string
	FrontendDir        string
	MigrationsDir      string
	EnableHSTS         bool
	Environment        string
	MaxWSConnections   int
	MaxPlayersPerRoom  int
	MetricsUser        string
	MetricsPassword    string
	OTLPEndpoint    string
	OTLPInsecure    bool
	OTELSampleRatio float64
	CloudRegion     string

	// EnableEmbeddedWorkers controls whether the server process starts async
	// workers (GameResult/Outbox/GDPR) in-process. Production default: true
	// (in-process; standalone game-worker binary removed per ADR-032).
	EnableEmbeddedWorkers bool
}

func Load() *Env {
	return &Env{
		JWTPrivateKey:         os.Getenv("JWT_PRIVATE_KEY"),
		JWTPublicKey:          os.Getenv("JWT_PUBLIC_KEY"),
		AdminJWTPrivateKey:    os.Getenv("ADMIN_JWT_PRIVATE_KEY"),
		AdminJWTPublicKey:     os.Getenv("ADMIN_JWT_PUBLIC_KEY"),
		DatabaseURL:           os.Getenv("DATABASE_URL"),
		RedisURL:              GetEnv("REDIS_URL", "localhost:6379"),
		RedisEphemeralURL:     GetEnv("REDIS_EPHEMERAL_URL", ""),
		RedisRegionURL:        GetEnv("REDIS_REGIONAL_URL", ""),
		RedisPubSubURL:        GetEnv("REDIS_PUBSUB_URL", ""),
		EncryptionKey:         os.Getenv("ENCRYPTION_KEY"),
		ResendAPIKey:          os.Getenv("RESEND_API_KEY"),
		EmailFrom:             os.Getenv("EMAIL_FROM"),
		AdminPassword:         os.Getenv("ADMIN_PASSWORD"),
		AuditSecret:           os.Getenv("AUDIT_SECRET"),
		TrustedProxyCIDRs:     os.Getenv("TRUSTED_PROXY_CIDRS"),
		AllowedOrigins:        os.Getenv("ALLOWED_ORIGINS"),
		Port:                  GetEnv("PORT", "8080"),
		FrontendDir:           os.Getenv("FRONTEND_DIR"),
		MigrationsDir:         GetEnv("MIGRATIONS_DIR", "migrations"),
		EnableHSTS:            !strings.EqualFold(os.Getenv("ENABLE_HSTS"), "false"),
		Environment:           os.Getenv("ENV"),
		MaxWSConnections:      GetEnvInt("MAX_WS_CONNECTIONS", MaxWSConnections),
		MaxPlayersPerRoom:     GetEnvInt("MAX_PLAYERS_PER_ROOM", MaxPlayersPerRoom),
		MetricsUser:           os.Getenv("METRICS_USER"),
		MetricsPassword:       os.Getenv("METRICS_PASSWORD"),
		OTLPEndpoint:          os.Getenv("OTLP_ENDPOINT"),
		OTLPInsecure:          strings.EqualFold(os.Getenv("OTLP_INSECURE"), "true"),
		OTELSampleRatio:       getEnvFloat64("OTEL_SAMPLE_RATIO", 0.1),
		CloudRegion:           os.Getenv("CLOUD_REGION"),
		EnableEmbeddedWorkers: !strings.EqualFold(os.Getenv("ENABLE_EMBEDDED_WORKERS"), "false") && os.Getenv("ENABLE_EMBEDDED_WORKERS") != "0",
	}
}

func (e *Env) IsProduction() bool {
	return e.Environment == EnvProduction
}

func (e *Env) Validate() error {
	var missing []string

	if e.JWTPrivateKey == "" {
		if e.IsProduction() {
			missing = append(missing, "JWT_PRIVATE_KEY")
		}
	} else if e.IsProduction() && isWeakJWTSecret(e.JWTPrivateKey) {
		return fmt.Errorf("JWT_PRIVATE_KEY contains a known weak/dev value; refuse to start in production mode (set ENV=production only for production)")
	}
	if e.IsProduction() && e.JWTPublicKey == "" {
		missing = append(missing, "JWT_PUBLIC_KEY")
	}
	if e.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if e.IsProduction() {
		if e.EncryptionKey == "" {
			missing = append(missing, "ENCRYPTION_KEY")
		}
		if e.AuditSecret == "" {
			missing = append(missing, "AUDIT_SECRET")
		}
		if err := validateDatabaseURLSSLModes(e.DatabaseURL); err != nil {
			return err
		}
	}
	if e.IsProduction() && strings.TrimSpace(e.TrustedProxyCIDRs) == "" {
		missing = append(missing, "TRUSTED_PROXY_CIDRS")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	if e.IsProduction() {
		if err := validateTrustedProxyCIDRs(e.TrustedProxyCIDRs); err != nil {
			return err
		}
	}
	return nil
}

func validateTrustedProxyCIDRs(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("TRUSTED_PROXY_CIDRS is empty")
	}
	valid := 0
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		entry := part
		if !strings.Contains(part, "/") {
			part += "/32"
		}
		if _, _, err := net.ParseCIDR(part); err != nil {
			return fmt.Errorf("TRUSTED_PROXY_CIDRS contains invalid CIDR %q", entry)
		}
		valid++
	}
	if valid == 0 {
		return fmt.Errorf("TRUSTED_PROXY_CIDRS contains no valid CIDR entries")
	}
	return nil
}

// GetRedisStatefulURL returns the Redis URL for stateful data.
// When REDIS_REGIONAL_URL is set (Phase 3 multi-region), it takes precedence for stateful data.
func (e *Env) GetRedisStatefulURL() string {
	if e.RedisRegionURL != "" {
		return e.RedisRegionURL
	}
	return e.RedisURL
}

// GetRedisEphemeralURL falls back to the stateful URL (single-instance mode) when REDIS_EPHEMERAL_URL is unset.
func (e *Env) GetRedisEphemeralURL() string {
	if e.RedisEphemeralURL != "" {
		return e.RedisEphemeralURL
	}
	return e.GetRedisStatefulURL()
}

func (e *Env) GetRedisPubSubURL() string {
	if e.RedisPubSubURL != "" {
		return e.RedisPubSubURL
	}
	return e.GetRedisStatefulURL()
}

func (e *Env) GetAdminJWTPrivateKey() string {
	if e.AdminJWTPrivateKey != "" {
		return e.AdminJWTPrivateKey
	}
	if e.IsProduction() {
		slog.Warn("ADMIN_JWT_PRIVATE_KEY not set in production - admin and user JWTs share the same signing key. Set ADMIN_JWT_PRIVATE_KEY to a separate key for defense-in-depth.")
	}
	return e.JWTPrivateKey
}

// AuditSecretOrJWT falls back to JWT_PRIVATE_KEY, but in production AUDIT_SECRET must be
// explicitly set - the fallback compromises audit integrity since a single key leak
// breaks both auth and audit.
func (e *Env) AuditSecretOrJWT() string {
	if e.AuditSecret != "" {
		return e.AuditSecret
	}
	if e.IsProduction() {
		slog.Error("AUDIT_SECRET not set in production - audit log integrity is compromised. Set AUDIT_SECRET to a separate key from JWT_PRIVATE_KEY.")
	}
	return e.JWTPrivateKey
}

func GetEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func GetEnvInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return n
}

func GetEnvIntPositive(key string, defaultVal int) int {
	n := GetEnvInt(key, defaultVal)
	if n <= 0 {
		return defaultVal
	}
	return n
}

// GetEnvDuration accepts Go duration strings ("5s", "1m", "500ms") via time.ParseDuration,
// or plain integers interpreted as seconds for backwards compatibility (v2-R-38).
// Non-positive durations and parse failures fall back to defaultVal.
func GetEnvDuration(key string, defaultVal time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	if d, err := time.ParseDuration(v); err == nil {
		if d > 0 {
			return d
		}
		return defaultVal
	}
	if seconds, err := strconv.Atoi(v); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return defaultVal
}

func isWeakJWTSecret(secret string) bool {
	if len(secret) < 32 {
		return true
	}
	return strings.Contains(secret, "DEV_ONLY") || strings.Contains(secret, "change-in-production")
}

func validateDatabaseURLSSLModes(dbURL string) error {
	if !strings.HasPrefix(dbURL, "postgres://") && !strings.HasPrefix(dbURL, "postgresql://") {
		return nil
	}
	u, err := url.Parse(dbURL)
	if err != nil {
		return fmt.Errorf("DATABASE_URL: %w", err)
	}
	sslmodes := u.Query()["sslmode"]
	if len(sslmodes) == 0 {
		return nil
	}
	finalSSLMode := sslmodes[len(sslmodes)-1]
	switch finalSSLMode {
	case "disable", "allow", "prefer":
		return fmt.Errorf("DATABASE_URL sslmode=%q rejected in production; use require, verify-ca, or verify-full", finalSSLMode)
	}
	return nil
}

func getEnvFloat64(key string, defaultVal float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return defaultVal
}

type RedisConn struct {
	Addr     string
	Password string
	DB       int // misc-022: Redis database number from URL path (e.g., redis://host:6379/2)
}

// ParseRedisURL accepts host:port or redis://[:password@]host:port[/db].
func ParseRedisURL(raw string) (RedisConn, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return RedisConn{}, fmt.Errorf("REDIS_URL is empty")
	}
	if !strings.HasPrefix(raw, "redis://") && !strings.HasPrefix(raw, "rediss://") {
		return RedisConn{Addr: raw}, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return RedisConn{}, fmt.Errorf("parse REDIS_URL: %w", err)
	}
	if u.Host == "" {
		return RedisConn{}, fmt.Errorf("parse REDIS_URL: missing host")
	}
	conn := RedisConn{Addr: u.Host}
	if u.User != nil {
		conn.Password, _ = u.User.Password()
	}
	// misc-022: Parse the DB number from the URL path (e.g., "/2" → DB=2).
	// Previously silently ignored, causing unexpected default-DB usage.
	if u.Path != "" && u.Path != "/" {
		dbStr := strings.TrimPrefix(u.Path, "/")
		db, err := strconv.Atoi(dbStr)
		if err != nil || db < 0 {
			return RedisConn{}, fmt.Errorf("parse REDIS_URL: invalid DB number %q", dbStr)
		}
		conn.DB = db
	}
	return conn, nil
}

// TimeoutConfig holds timeout durations for various operations.
//
// Enterprise rationale: Hardcoded timeouts cannot be tuned for different
// environments (dev vs staging vs prod). Connection, read, and request
// timeouts serve different purposes and must be independently configurable:
// - Connect timeout: TCP handshake time (network RTT)
// - Read timeout: waiting for first byte of response (server processing)
// - Request timeout: total time including retries (user-facing SLA)
// Trade-off: More config = more complexity, but the alternative is
// redeploying to change a timeout value.
type TimeoutConfig struct {
	PGConnectTimeout time.Duration
	PGQueryTimeout   time.Duration
	PGRequestTimeout time.Duration

	RedisConnectTimeout time.Duration
	RedisReadTimeout    time.Duration
	RedisWriteTimeout   time.Duration

	HTTPConnectTimeout time.Duration
	HTTPRequestTimeout time.Duration

	WSWriteTimeout   time.Duration
	WSPongTimeout    time.Duration
	WSPingInterval   time.Duration
	WSHandlerTimeout time.Duration
}

func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		PGConnectTimeout: GetEnvDuration("PG_CONNECT_TIMEOUT", 5*time.Second),
		PGQueryTimeout:   GetEnvDuration("PG_QUERY_TIMEOUT", 10*time.Second),
		PGRequestTimeout: GetEnvDuration("PG_REQUEST_TIMEOUT", 30*time.Second),

		RedisConnectTimeout: GetEnvDuration("REDIS_CONNECT_TIMEOUT", 3*time.Second),
		RedisReadTimeout:    GetEnvDuration("REDIS_READ_TIMEOUT", 3*time.Second),
		RedisWriteTimeout:   GetEnvDuration("REDIS_WRITE_TIMEOUT", 3*time.Second),

		HTTPConnectTimeout: GetEnvDuration("HTTP_CONNECT_TIMEOUT", 5*time.Second),
		HTTPRequestTimeout: GetEnvDuration("HTTP_REQUEST_TIMEOUT", 10*time.Second),

		WSWriteTimeout:   GetEnvDuration("WS_WRITE_TIMEOUT", 10*time.Second),
		WSPongTimeout:    GetEnvDuration("WS_PONG_TIMEOUT", 60*time.Second),
		WSPingInterval:   GetEnvDuration("WS_PING_INTERVAL", 30*time.Second),
		WSHandlerTimeout: GetEnvDuration("WS_HANDLER_TIMEOUT", 2*time.Hour),
	}
}
