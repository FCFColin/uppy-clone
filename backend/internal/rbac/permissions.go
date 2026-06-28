// Package rbac provides lightweight role-based access control middleware.
package rbac

// permissions holds the in-memory RBAC policy (ADR-026 lightweight RBAC).
var permissions = map[string]map[string][]string{
	RoleAdmin: {
		"config":    {"read", "write"},
		"users":     {"read", "write"},
		"lobby":     {"create", "join", "read"},
		"user_data": {"read", "delete"},
	},
	RoleModerator: {
		"config": {"read"},
		"users":  {"read"},
		"lobby":  {"read"},
	},
	RoleUser: {
		"lobby":     {"create", "join", "read"},
		"user_data": {"read", "delete"},
	},
	RoleGuest: {
		"lobby": {"read"},
	},
}
