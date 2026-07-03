// Package handler implements HTTP and WebSocket API endpoints.
package handler

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	db     UserStore
	redis  TokenStore
	config *Config
	auth   AuthService
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(db UserStore, redis TokenStore, auth AuthService, config *Config) *AuthHandler {
	return &AuthHandler{
		db:     db,
		redis:  redis,
		auth:   auth,
		config: config,
	}
}
