package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Env holds server environment configuration loaded from process env.
type Env struct {
	JWTSecret         string
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
	if e.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if e.EncryptionKey == "" {
		missing = append(missing, "ENCRYPTION_KEY")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return nil
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
