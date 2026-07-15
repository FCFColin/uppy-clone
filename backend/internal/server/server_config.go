package server

import (
	"log/slog"
	"os"
	"strings"

	appConfig "github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/handler"
)

var serverEnv *appConfig.Env

// loadConfig loads configuration from environment variables.
func loadConfig() *handler.Config {
	serverEnv = appConfig.Load()
	return &handler.Config{
		ResendAPIKey:       serverEnv.ResendAPIKey,
		EmailFrom:          serverEnv.EmailFrom,
		AdminPassword:      serverEnv.AdminPassword,
		JWTPrivateKey:      serverEnv.JWTPrivateKey,
		JWTPublicKey:       serverEnv.JWTPublicKey,
		AdminJWTPrivateKey: serverEnv.GetAdminJWTPrivateKey(),
		AdminJWTPublicKey:  serverEnv.AdminJWTPublicKey,
		DatabaseURL:        serverEnv.DatabaseURL,
		RedisURL:           serverEnv.GetRedisStatefulURL(),
		RedisEphemeralURL:  serverEnv.GetRedisEphemeralURL(),
		RedisPubSubURL:     serverEnv.GetRedisPubSubURL(),
		Port:               serverEnv.Port,
		FrontendDir:        serverEnv.FrontendDir,
	}
}

// validateConfig validates required config fields and rejects weak dev secrets in production.
func validateConfig(_ *handler.Config, logger *slog.Logger) {
	if err := serverEnv.Validate(); err != nil {
		logger.Error("configuration validation failed", "error", err)
		exitFunc(1)
	}
}

// initCrypto initializes the AES encryption key from the environment.
func initCrypto(_ *handler.Config) error {
	return crypto.InitFromEnv()
}

// getEnv returns the environment variable value, preferring loaded serverEnv when set.
func getEnv(key, defaultVal string) string {
	if serverEnv != nil {
		switch key {
		case "ALLOWED_ORIGINS":
			if serverEnv.AllowedOrigins != "" {
				return serverEnv.AllowedOrigins
			}
		case "PORT":
			if serverEnv.Port != "" {
				return serverEnv.Port
			}
		}
	}
	return appConfig.GetEnv(key, defaultVal)
}

// getEnvInt returns the environment variable value as int, preferring loaded serverEnv when set.
func getEnvInt(key string, defaultVal int) int {
	if serverEnv != nil {
		switch key {
		case "MAX_WS_CONNECTIONS":
			if serverEnv.MaxWSConnections != 0 {
				return serverEnv.MaxWSConnections
			}
		case "MAX_PLAYERS_PER_ROOM":
			if serverEnv.MaxPlayersPerRoom != 0 {
				return serverEnv.MaxPlayersPerRoom
			}
		}
	}
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return appConfig.GetEnvInt(key, defaultVal)
}

// parseLogLevel converts a log level string to slog.Level.
func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
