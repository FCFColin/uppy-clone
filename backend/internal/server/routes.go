// Package server wires HTTP routes, middleware, and the application lifecycle.
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
func setupRoutes(r *chi.Mux, authHandler *handler.AuthHandler, lobbyHandler *handler.LobbyHandler, adminHandler *handler.AdminHandler, statsHandler *handler.StatsHandler, jwtMgr *auth.JWTManager, db *store.PostgresStore, cluster *store.RedisCluster, rbacEnforcer *rbac.Enforcer, cfg *handler.Config, hub *game.Hub) {
	setupMiddleware(r)
	setupHealthAndMetricsRoutes(r, db, cluster, hub)
	setupAuthRoutes(r, authHandler, cluster, jwtMgr, rbacEnforcer)
	setupStatsRoutes(r, statsHandler, cluster, jwtMgr, rbacEnforcer)
	setupLobbyRoutes(r, lobbyHandler, cluster, jwtMgr, rbacEnforcer)
	setupAdminRoutes(r, adminHandler, cluster, jwtMgr, rbacEnforcer)
	setupStaticRoutes(r, cfg)
}
