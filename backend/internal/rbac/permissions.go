// Package rbac provides lightweight role-based access control middleware.
package rbac

import "github.com/uppy-clone/backend/internal/domain"

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
