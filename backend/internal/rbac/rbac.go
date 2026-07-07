package rbac

import (
	"log/slog"
	"net/http"

	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/slogctx"
)

// Roles
const (
	RoleAdmin     = "admin"
	RoleModerator = "moderator"
	RoleUser      = "user"
	RoleGuest     = "guest"
)

// Enforcer checks RBAC permissions from an in-memory policy map.
type Enforcer struct{}

// NewEnforcer creates the default RBAC enforcer (replaces Casbin file adapter).
func NewEnforcer() *Enforcer {
	return &Enforcer{}
}

// CheckPermission checks if a user with the given role can access the resource.
func (e *Enforcer) CheckPermission(role, resource, action string) bool {
	resources, ok := permissions[role]
	if !ok {
		return false
	}
	actions, ok := resources[resource]
	if !ok {
		return false
	}
	for _, a := range actions {
		if a == action {
			return true
		}
	}
	return false
}

// Middleware returns an HTTP middleware that checks RBAC permissions.
func (e *Enforcer) Middleware(resource, action string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, ok := domain.ContextKeyRole.Value(r.Context())
			if !ok || role == "" {
				role = RoleGuest
			}
			{
				logger := slogctx.LoggerFromContext(r.Context())
				logger = logger.With("role", role)
				ctx := slogctx.WithLogger(r.Context(), logger)
				r = r.WithContext(ctx)
			}
			if !e.CheckPermission(role, resource, action) {
				slog.Warn("RBAC denied", "role", role, "resource", resource, "action", action)
				audit.Log(r.Context(), audit.AuditEntry{
					Action:   "rbac.deny",
					ActorID:  role,
					Resource: resource,
					Before:   map[string]interface{}{"action": action},
				})
				http.Error(w, `{"type":"https://httpstatuses.com/403","title":"Forbidden","status":403,"detail":"insufficient permissions"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
