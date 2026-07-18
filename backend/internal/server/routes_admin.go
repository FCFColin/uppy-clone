package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/handler"
	appMiddleware "github.com/uppy-clone/backend/internal/middleware"
	"github.com/uppy-clone/backend/internal/rbac"
	"github.com/uppy-clone/backend/internal/store"
)

// setupAdminRoutes registers admin login and admin-protected config routes.
// Rate limiting uses ephemeral Redis (ADR-029).
func setupAdminRoutes(r *chi.Mux, adminHandler *handler.AdminHandler, cluster *store.RedisCluster, jwtMgr *auth.JWTManager, rbacEnforcer *rbac.Enforcer) {
	r.Route("/api/v1/admin", func(r chi.Router) {
		r.With(appMiddleware.EndpointRateLimit(cluster.Ephemeral, "admin:login", jwtMgr)).Post("/login", adminHandler.Login)

		r.Group(func(r chi.Router) {
			r.Use(adminAuthMiddleware(adminHandler))
			r.Post("/logout", adminHandler.Logout)
			r.With(rbacEnforcer.Middleware("config", "read")).Get("/config", adminHandler.GetConfig)
			r.With(rbacEnforcer.Middleware("config", "write")).Patch("/config", adminHandler.UpdateConfig)
			r.With(rbacEnforcer.Middleware("config", "write")).Put("/config", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Deprecation", "true")
				w.Header().Set("Sunset", "Sat, 01 Jan 2027 00:00:00 GMT")
				w.Header().Set("Link", "</api/v1/admin/config>; rel=\"successor-version\"")
				adminHandler.UpdateConfig(w, r)
			})
		})
	})
}
