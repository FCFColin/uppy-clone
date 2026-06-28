// Package handler implements HTTP and WebSocket API endpoints.
package handler

import (
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/store"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	jwtMgr     *auth.JWTManager
	refreshMgr *auth.RefreshTokenManager
	db         *store.PostgresStore
	redis      *store.RedisStore
	config     *Config
	magicLink  *auth.MagicLinkService
	timeouts   config.TimeoutConfig
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(jwtMgr *auth.JWTManager, refreshMgr *auth.RefreshTokenManager, db *store.PostgresStore, redis *store.RedisStore, config *Config, timeouts config.TimeoutConfig) *AuthHandler {
	return &AuthHandler{
		jwtMgr:     jwtMgr,
		refreshMgr: refreshMgr,
		db:         db,
		redis:      redis,
		config:     config,
		magicLink:  auth.NewMagicLinkService(),
		timeouts:   timeouts,
	}
}
