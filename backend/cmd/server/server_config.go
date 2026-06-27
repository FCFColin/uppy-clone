package main

import (
	"log/slog"
	"os"
	"strconv"
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
		ResendAPIKey:  serverEnv.ResendAPIKey,
		EmailFrom:     serverEnv.EmailFrom,
		AdminPassword: serverEnv.AdminPassword,
		JWTSecret:     serverEnv.JWTSecret,
		DatabaseURL:   serverEnv.DatabaseURL,
		RedisURL:      serverEnv.RedisURL,
		Port:          serverEnv.Port,
		FrontendDir:   serverEnv.FrontendDir,
	}
}

// validateConfig validates required config fields and rejects weak dev secrets in production.
func validateConfig(cfg *handler.Config, logger *slog.Logger) {
	if err := serverEnv.Validate(); err != nil {
		logger.Error("configuration validation failed", "error", err)
		os.Exit(1)
	}
	_ = cfg
}

// initCrypto initializes the AES encryption key from the environment.
func initCrypto(_ *handler.Config) error {
	return crypto.InitFromEnv()
}

// getEnv returns the environment variable value or a default.
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
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// getEnvInt returns the environment variable value as int, or a default.
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
	n, err := strconv.Atoi(val)
	if err != nil {
		slog.Warn("invalid env var, using default", "key", key, "value", val, "default", defaultVal)
		return defaultVal
	}
	return n
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
