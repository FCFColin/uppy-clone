package rbac

import (
	"log/slog"
	"net/http"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	fileadapter "github.com/casbin/casbin/v2/persist/file-adapter"

	"github.com/uppy-clone/backend/internal/auth"
)

// Enterprise rationale: RBAC (Role-Based Access Control) is the most common
// authorization model in enterprises. It maps naturally to organizational
// roles (admin, moderator, user). Trade-off: Less flexible than ABAC for
// attribute-based rules, but simpler to reason about and audit.

// Roles
const (
	RoleAdmin     = "admin"
	RoleModerator = "moderator"
	RoleUser      = "user"
	RoleGuest     = "guest"
)

// Enforcer wraps casbin enforcer with our RBAC model.
type Enforcer struct {
	*casbin.Enforcer
}

// NewEnforcer creates a RBAC enforcer from model and policy files.
func NewEnforcer(modelPath, policyPath string) (*Enforcer, error) {
	m, err := model.NewModelFromFile(modelPath)
	if err != nil {
		return nil, err
	}
	a := fileadapter.NewAdapter(policyPath)
	e, err := casbin.NewEnforcer(m, a)
	if err != nil {
		return nil, err
	}
	return &Enforcer{Enforcer: e}, nil
}

// CheckPermission checks if a user with the given role can access the resource.
func (e *Enforcer) CheckPermission(role, resource, action string) bool {
	ok, _ := e.Enforce(role, resource, action)
	return ok
}

// Middleware returns an HTTP middleware that checks RBAC permissions.
//
// 企业为何需要：角色必须来自已验证的凭据（JWT claims/已通过认证的中间件），
// 而非客户端可控的 HTTP 头。X-User-Role 头可被任意客户端伪造，导致权限提升。
// 此处从 request context 读取由认证中间件（auth.AuthMiddleware / adminAuthMiddleware）
// 在验证凭据后注入的角色，确保角色来源不可被客户端伪造。
func (e *Enforcer) Middleware(resource, action string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, ok := auth.RoleFromContext(r)
			if !ok || role == "" {
				role = RoleGuest
			}
			if !e.CheckPermission(role, resource, action) {
				slog.Warn("RBAC denied", "role", role, "resource", resource, "action", action)
				http.Error(w, `{"type":"https://httpstatuses.com/403","title":"Forbidden","status":403,"detail":"insufficient permissions"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
