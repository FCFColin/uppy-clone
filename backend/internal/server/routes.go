package server

import (
	"github.com/go-chi/chi/v5"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/handler"
	"github.com/uppy-clone/backend/internal/rbac"
	"github.com/uppy-clone/backend/internal/store"
)

// setupRoutes registers all HTTP routes and middleware on the chi router.
func setupRoutes(r *chi.Mux, authHandler *handler.AuthHandler, lobbyHandler *handler.LobbyHandler, adminHandler *handler.AdminHandler, jwtMgr *auth.JWTManager, db *store.PostgresStore, redis *store.RedisStore, rbacEnforcer *rbac.Enforcer, cfg *handler.Config, hub *game.Hub) {
	setupMiddleware(r)
	setupHealthAndMetricsRoutes(r, db, redis, hub)
	setupAuthRoutes(r, authHandler, redis, jwtMgr, rbacEnforcer)
	setupLobbyRoutes(r, lobbyHandler, redis, jwtMgr, rbacEnforcer)
	setupAdminRoutes(r, adminHandler, redis, jwtMgr, rbacEnforcer)
	setupStaticRoutes(r, cfg)
}
