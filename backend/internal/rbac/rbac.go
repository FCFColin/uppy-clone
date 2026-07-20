// Package rbac provides lightweight role-based access control middleware.
package rbac

import (
	"log/slog"
	"net/http"

	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/util"
)

// Roles
const (
	RoleGuest = "guest"
)

// RBAC action verbs shared across the permissions table.
const (
	ActionRead   = "read"
	ActionWrite  = "write"
	ActionCreate = "create"
	ActionJoin   = "join"
	ActionDelete = "delete"
)

// RBAC resource keys shared across the permissions table.
const (
	ResourceConfig   = "config"
	ResourceUsers    = "users"
	ResourceLobby    = "lobby"
	ResourceUserData = "user_data"
)

// permissions holds the in-memory RBAC policy (ADR-026 lightweight RBAC).
//
// auth-022: This map is intentionally a package-level var for the following
// safety reasons:
//   - It is unexported (lowercase), so external packages cannot access or
//     mutate it.
//   - It is never mutated at runtime — policy changes require a code change
//     and redeployment.
//   - It is only read by (*Enforcer).CheckPermission, which performs
//     read-only lookups.
//   - Go does not support const maps; using a var with a clear "do not mutate"
//     contract is the idiomatic pattern (cf. database/sql drivers registry).
//
// If runtime-mutable permissions are needed in the future, wrap this in a
// sync.RWMutex-protected accessor instead of mutating the var directly.
//
//nolint:gochecknoglobals // intentional immutable-by-convention policy table
var permissions = map[string]map[string][]string{
	domain.RoleAdmin: {
		ResourceConfig:   {ActionRead, ActionWrite},
		ResourceUsers:    {ActionRead, ActionWrite},
		ResourceLobby:    {ActionCreate, ActionJoin, ActionRead},
		ResourceUserData: {ActionRead, ActionDelete},
	},
	domain.RoleUser: {
		ResourceLobby:    {ActionCreate, ActionJoin, ActionRead},
		ResourceUserData: {ActionRead, ActionDelete},
	},
	RoleGuest: {
		ResourceLobby: {ActionRead},
	},
}

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
				logger := util.LoggerFromContext(r.Context())
				logger = logger.With("role", role)
				ctx := util.WithLogger(r.Context(), logger)
				r = r.WithContext(ctx)
			}
			if !e.CheckPermission(role, resource, action) {
				slog.Warn("RBAC denied", "role", role, "resource", resource, "action", action)
				actorID := role
				actorType := audit.ActorTypeAnonymous
				if uid, ok := domain.ContextKeyUserID.Value(r.Context()); ok && uid != "" {
					actorID = uid
					actorType = audit.ActorTypeUser
				}
				audit.Log(r.Context(), audit.AuditEntry{
					Action:    "rbac.deny",
					ActorType: actorType,
					ActorID:   actorID,
					Resource:  resource,
					Before:    map[string]interface{}{"role": role, "action": action},
				})
				domain.Forbidden("insufficient permissions").Write(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
