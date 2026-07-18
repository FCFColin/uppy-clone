package domain

import "context"

// ContextKey is a typed key for storing auth values in request contexts.
type ContextKey string

const (
	// ContextKeyUserID stores the authenticated user's ID.
	ContextKeyUserID ContextKey = "auth_user_id"
	// ContextKeyNickname stores the authenticated user's nickname.
	ContextKeyNickname ContextKey = "auth_nickname"
	// ContextKeyRole stores the authenticated user's role.
	ContextKeyRole ContextKey = "auth_user_role"
	// ContextKeyJTI stores the JWT ID for revocation checks.
	ContextKeyJTI ContextKey = "auth_jti"
)

// WithValue returns a new context with the given string value stored under this key.
func (k ContextKey) WithValue(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, k, v)
}

// Value retrieves a string value stored under this key, returning ok=false if absent.
func (k ContextKey) Value(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(k).(string)
	return v, ok
}

// Role constants used in JWT claims and context injection.
const (
	// RoleUser is the role assigned to standard authenticated users.
	RoleUser = "user"
	// RoleAdmin is the role assigned to administrators with elevated privileges.
	RoleAdmin = "admin"
)

// WithRole returns a new context with the given role value.
// 企业为何需要：角色必须来自已验证的凭据（JWT claims/已通过认证的中间件），
// 而非客户端可控的 HTTP 头。X-User-Role 头可被任意客户端伪造，导致权限提升。
// 该函数供认证中间件在验证凭据后注入角色，RBAC 中间件再从 context 读取。
func WithRole(ctx context.Context, role string) context.Context {
	return ContextKeyRole.WithValue(ctx, role)
}

// RoleFromContext extracts the role from the context (set by auth middleware).
// Returns the role and ok=true if a role was set by an authenticated middleware.
func RoleFromContext(ctx context.Context) (string, bool) {
	return ContextKeyRole.Value(ctx)
}

type proxyKey struct{}

// WithTrustedProxy marks whether the request peer is a trusted reverse proxy.
func WithTrustedProxy(ctx context.Context, trusted bool) context.Context {
	return context.WithValue(ctx, proxyKey{}, trusted)
}

// IsTrustedProxy reports whether X-Forwarded-* headers were honored for this request.
func IsTrustedProxy(ctx context.Context) bool {
	v, ok := ctx.Value(proxyKey{}).(bool)
	return ok && v
}
