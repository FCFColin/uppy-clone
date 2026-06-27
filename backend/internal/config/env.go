package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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
		RedisURL:          getEnv("REDIS_URL", "localhost:6379"),
		EncryptionKey:     os.Getenv("ENCRYPTION_KEY"),
		ResendAPIKey:      os.Getenv("RESEND_API_KEY"),
		EmailFrom:         os.Getenv("EMAIL_FROM"),
		AdminPassword:     os.Getenv("ADMIN_PASSWORD"),
		AuditSecret:       os.Getenv("AUDIT_SECRET"),
		TrustedProxyCIDRs: os.Getenv("TRUSTED_PROXY_CIDRS"),
		AllowedOrigins:    os.Getenv("ALLOWED_ORIGINS"),
		Port:              getEnv("PORT", "8080"),
		FrontendDir:       os.Getenv("FRONTEND_DIR"),
		EnableHSTS:        os.Getenv("ENABLE_HSTS") != "false",
		MaxWSConnections:  getEnvInt("MAX_WS_CONNECTIONS", MaxWSConnections),
		MaxPlayersPerRoom: getEnvInt("MAX_PLAYERS_PER_ROOM", MaxPlayersPerRoom),
		MetricsUser:       os.Getenv("METRICS_USER"),
		MetricsPassword:   os.Getenv("METRICS_PASSWORD"),
	}
}

// Validate returns an error listing all missing or invalid required fields.
func (e *Env) Validate() error {
	var missing []string

	if e.JWTSecret == "" {
		missing = append(missing, "JWT_SECRET")
	} else if e.EnableHSTS &&
		(strings.Contains(e.JWTSecret, "DEV_ONLY") || strings.Contains(e.JWTSecret, "change-in-production")) {
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

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
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
