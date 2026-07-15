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

// Env holds server environment configuration loaded from process env.
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
}

// Load reads configuration from environment variables.
func Load() *Env {
	return &Env{
		JWTPrivateKey:      os.Getenv("JWT_PRIVATE_KEY"),
		JWTPublicKey:       os.Getenv("JWT_PUBLIC_KEY"),
		AdminJWTPrivateKey: os.Getenv("ADMIN_JWT_PRIVATE_KEY"),
		AdminJWTPublicKey:  os.Getenv("ADMIN_JWT_PUBLIC_KEY"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		RedisURL:           GetEnv("REDIS_URL", "localhost:6379"),
		RedisEphemeralURL:  GetEnv("REDIS_EPHEMERAL_URL", ""),
		RedisRegionURL:     GetEnv("REDIS_REGIONAL_URL", ""),
		RedisPubSubURL:     GetEnv("REDIS_PUBSUB_URL", ""),
		EncryptionKey:      os.Getenv("ENCRYPTION_KEY"),
		ResendAPIKey:       os.Getenv("RESEND_API_KEY"),
		EmailFrom:          os.Getenv("EMAIL_FROM"),
		AdminPassword:      os.Getenv("ADMIN_PASSWORD"),
		AuditSecret:        os.Getenv("AUDIT_SECRET"),
		TrustedProxyCIDRs:  os.Getenv("TRUSTED_PROXY_CIDRS"),
		AllowedOrigins:     os.Getenv("ALLOWED_ORIGINS"),
		Port:               GetEnv("PORT", "8080"),
		FrontendDir:        os.Getenv("FRONTEND_DIR"),
		MigrationsDir:      GetEnv("MIGRATIONS_DIR", "migrations"),
		EnableHSTS:         !strings.EqualFold(os.Getenv("ENABLE_HSTS"), "false"),
		Environment:        os.Getenv("ENV"),
		MaxWSConnections:   GetEnvInt("MAX_WS_CONNECTIONS", MaxWSConnections),
		MaxPlayersPerRoom:  GetEnvInt("MAX_PLAYERS_PER_ROOM", MaxPlayersPerRoom),
		MetricsUser:        os.Getenv("METRICS_USER"),
		MetricsPassword:    os.Getenv("METRICS_PASSWORD"),
	}
}

// IsProduction returns true when the environment is set to production.
func (e *Env) IsProduction() bool {
	return e.Environment == "production"
}

// Validate returns an error listing all missing or invalid required fields.
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

// GetRedisStatefulURL returns the Redis URL for stateful data (room registry, auth tokens).
// When REDIS_REGIONAL_URL is set (Phase 3 multi-region), it takes precedence for stateful data.
func (e *Env) GetRedisStatefulURL() string {
	if e.RedisRegionURL != "" {
		return e.RedisRegionURL
	}
	return e.RedisURL
}

// GetRedisEphemeralURL returns the Redis URL for ephemeral data (rate limiting, cache).
// When REDIS_EPHEMERAL_URL is unset, falls back to the stateful URL (single-instance mode).
func (e *Env) GetRedisEphemeralURL() string {
	if e.RedisEphemeralURL != "" {
		return e.RedisEphemeralURL
	}
	return e.GetRedisStatefulURL()
}

// GetRedisPubSubURL returns the Redis Pub/Sub URL, defaulting to the stateful Redis URL when empty.
func (e *Env) GetRedisPubSubURL() string {
	if e.RedisPubSubURL != "" {
		return e.RedisPubSubURL
	}
	return e.GetRedisStatefulURL()
}

// GetAdminJWTPrivateKey returns ADMIN_JWT_PRIVATE_KEY or falls back to JWT_PRIVATE_KEY.
func (e *Env) GetAdminJWTPrivateKey() string {
	if e.AdminJWTPrivateKey != "" {
		return e.AdminJWTPrivateKey
	}
	if e.IsProduction() {
		slog.Warn("ADMIN_JWT_PRIVATE_KEY not set in production - admin and user JWTs share the same signing key. Set ADMIN_JWT_PRIVATE_KEY to a separate key for defense-in-depth.")
	}
	return e.JWTPrivateKey
}

// AuditSecretOrJWT returns AUDIT_SECRET or falls back to JWT_PRIVATE_KEY.
// In production, AUDIT_SECRET must be explicitly set - the fallback to JWTPrivateKey
// compromises audit integrity since a single key leak breaks both auth and audit.
func (e *Env) AuditSecretOrJWT() string {
	if e.AuditSecret != "" {
		return e.AuditSecret
	}
	if e.IsProduction() {
		slog.Error("AUDIT_SECRET not set in production - audit log integrity is compromised. Set AUDIT_SECRET to a separate key from JWT_PRIVATE_KEY.")
	}
	return e.JWTPrivateKey
}

// GetEnv returns the environment variable value or a default.
func GetEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// GetEnvInt returns the environment variable value as int, or a default.
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

// GetEnvIntPositive returns GetEnvInt but falls back to defaultVal when the value is <= 0.
func GetEnvIntPositive(key string, defaultVal int) int {
	n := GetEnvInt(key, defaultVal)
	if n <= 0 {
		return defaultVal
	}
	return n
}

// GetEnvDuration returns the environment variable value as duration, or a default.
//
// Accepted formats (unified behavior, see v2-R-38):
//   - Go duration string with unit suffix (e.g. "5s", "1m", "500ms") via time.ParseDuration.
//   - Plain integer interpreted as seconds (e.g. "10" → 10*time.Second) for backwards
//     compatibility with operators that historically supplied bare seconds.
//
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
