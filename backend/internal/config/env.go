package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Env holds server environment configuration loaded from process env.
type Env struct {
	JWTSecret         string
	AdminJWTSecret    string
	DatabaseURL       string
	RedisURL          string
	EncryptionKey     string
	ResendAPIKey      string
	EmailFrom         string
	AdminPassword     string
	AuditSecret       string
	TrustedProxyCIDRs string
	AllowedOrigins    string
	Port              string
	FrontendDir       string
	MigrationsDir     string
	EnableHSTS        bool
	MaxWSConnections  int
	MaxPlayersPerRoom int
	MetricsUser       string
	MetricsPassword   string
}

// Load reads configuration from environment variables.
func Load() *Env {
	return &Env{
		JWTSecret:         os.Getenv("JWT_SECRET"),
		AdminJWTSecret:    os.Getenv("ADMIN_JWT_SECRET"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		RedisURL:          GetEnv("REDIS_URL", "localhost:6379"),
		EncryptionKey:     os.Getenv("ENCRYPTION_KEY"),
		ResendAPIKey:      os.Getenv("RESEND_API_KEY"),
		EmailFrom:         os.Getenv("EMAIL_FROM"),
		AdminPassword:     os.Getenv("ADMIN_PASSWORD"),
		AuditSecret:       os.Getenv("AUDIT_SECRET"),
		TrustedProxyCIDRs: os.Getenv("TRUSTED_PROXY_CIDRS"),
		AllowedOrigins:    os.Getenv("ALLOWED_ORIGINS"),
		Port:              GetEnv("PORT", "8080"),
		FrontendDir:       os.Getenv("FRONTEND_DIR"),
		MigrationsDir:     GetEnv("MIGRATIONS_DIR", "migrations"),
		EnableHSTS:        os.Getenv("ENABLE_HSTS") != "false",
		MaxWSConnections:  GetEnvInt("MAX_WS_CONNECTIONS", MaxWSConnections),
		MaxPlayersPerRoom: GetEnvInt("MAX_PLAYERS_PER_ROOM", MaxPlayersPerRoom),
		MetricsUser:       os.Getenv("METRICS_USER"),
		MetricsPassword:   os.Getenv("METRICS_PASSWORD"),
	}
}

// Validate returns an error listing all missing or invalid required fields.
func (e *Env) Validate() error {
	var missing []string

	if e.JWTSecret == "" {
		missing = append(missing, "JWT_SECRET")
	} else if e.EnableHSTS && isWeakJWTSecret(e.JWTSecret) {
		return fmt.Errorf("JWT_SECRET contains a known weak/dev value; refuse to start in production mode (set ENABLE_HSTS=false only for local dev)")
	}
	if e.EnableHSTS && e.AdminJWTSecret == "" {
		missing = append(missing, "ADMIN_JWT_SECRET")
	} else if e.AdminJWTSecret != "" && len(e.AdminJWTSecret) < 32 {
		return fmt.Errorf("ADMIN_JWT_SECRET must be at least 32 bytes (256 bits) for HS256")
	}
	if e.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if e.EncryptionKey == "" {
		missing = append(missing, "ENCRYPTION_KEY")
	}
	if e.EnableHSTS && strings.TrimSpace(e.TrustedProxyCIDRs) == "" {
		missing = append(missing, "TRUSTED_PROXY_CIDRS")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	if e.EnableHSTS {
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

// AdminJWTSecretOrUser returns ADMIN_JWT_SECRET, falling back to JWT_SECRET for local dev.
func (e *Env) AdminJWTSecretOrUser() string {
	if e.AdminJWTSecret != "" {
		return e.AdminJWTSecret
	}
	return e.JWTSecret
}

// AuditSecretOrJWT returns AUDIT_SECRET or falls back to JWT_SECRET.
func (e *Env) AuditSecretOrJWT() string {
	if e.AuditSecret != "" {
		return e.AuditSecret
	}
	return e.JWTSecret
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
func GetEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return defaultVal
}

func isWeakJWTSecret(secret string) bool {
	return strings.Contains(secret, "DEV_ONLY") || strings.Contains(secret, "change-in-production")
}
