package rbac

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uppy-clone/backend/internal/auth"
)

// contextKey is a dedicated context key type for tests.
type contextKey struct{}

// testHandler 是一个简单的 handler，记录是否被调用。
func testHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}
}

// TestXUserRoleHeaderSpoofingDenied 验证设置 X-User-Role: admin 头部
// 不会在 JWT 为普通用户时授予 admin 权限。
// 企业为何需要：角色必须来自已验证的凭据（JWT claims），而非客户端可控输入。
// 否则任何用户可伪造 X-User-Role: admin 绕过授权。
func TestXUserRoleHeaderSpoofingDenied(t *testing.T) {
	t.Parallel()

	enforcer, err := NewEnforcer("model.conf", "policy.csv")
	if err != nil {
		t.Fatalf("NewEnforcer() error = %v", err)
	}

	// 模拟已认证的普通用户：context 中 role=user
	handler := enforcer.Middleware("config", "read")(testHandler())

	// 伪造 X-User-Role: admin 头部
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config", nil)
	req.Header.Set("X-User-Role", "admin")
	// 但 context 中实际角色是 "user"（由 auth middleware 注入）
	ctx := auth.WithRole(req.Context(), "user")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// 即使伪造了 X-User-Role: admin，普通用户仍应被拒绝
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status 403 for spoofed X-User-Role, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// TestValidAdminRoleFromContext 验证 context 中 role=admin 的合法请求
// 能通过 RBAC 检查。
func TestValidAdminRoleFromContext(t *testing.T) {
	t.Parallel()

	enforcer, err := NewEnforcer("model.conf", "policy.csv")
	if err != nil {
		t.Fatalf("NewEnforcer() error = %v", err)
	}

	handler := enforcer.Middleware("config", "read")(testHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config", nil)
	// 合法 admin 角色，由认证中间件注入 context
	ctx := auth.WithRole(req.Context(), "admin")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 for valid admin role, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// TestGuestDeniedForAdminResource 验证无角色（未认证）用户
// 访问 admin 资源时被拒绝。
func TestGuestDeniedForAdminResource(t *testing.T) {
	t.Parallel()

	enforcer, err := NewEnforcer("model.conf", "policy.csv")
	if err != nil {
		t.Fatalf("NewEnforcer() error = %v", err)
	}

	handler := enforcer.Middleware("config", "read")(testHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config", nil)
	// 不设置任何角色 — 模拟未认证用户

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status 403 for guest on admin resource, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// TestUserRoleCanAccessLobby 验证普通用户可以访问 lobby 资源。
func TestUserRoleCanAccessLobby(t *testing.T) {
	t.Parallel()

	enforcer, err := NewEnforcer("model.conf", "policy.csv")
	if err != nil {
		t.Fatalf("NewEnforcer() error = %v", err)
	}

	handler := enforcer.Middleware("lobby", "create")(testHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/registry/create", nil)
	ctx := auth.WithRole(req.Context(), "user")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 for user on lobby resource, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// TestSpoofedHeaderDoesNotOverrideContext 验证即使同时设置了
// X-User-Role 头部和 context 角色，RBAC 只看 context。
func TestSpoofedHeaderDoesNotOverrideContext(t *testing.T) {
	t.Parallel()

	enforcer, err := NewEnforcer("model.conf", "policy.csv")
	if err != nil {
		t.Fatalf("NewEnforcer() error = %v", err)
	}

	// 请求 config:read — 只有 admin 有权限
	handler := enforcer.Middleware("config", "read")(testHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config", nil)
	req.Header.Set("X-User-Role", "admin")
	// context 中是 user，RBAC 应只看 context
	ctx := auth.WithRole(req.Context(), "user")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 — RBAC must ignore X-User-Role header, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// TestNoRoleDefaultsToGuest 验证 context 中无角色时默认为 guest。
func TestNoRoleDefaultsToGuest(t *testing.T) {
	t.Parallel()

	enforcer, err := NewEnforcer("model.conf", "policy.csv")
	if err != nil {
		t.Fatalf("NewEnforcer() error = %v", err)
	}

	// guest 可以 lobby:read
	handler := enforcer.Middleware("lobby", "read")(testHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/registry/lobbies", nil)
	// 无角色 — 应默认 guest

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for guest on lobby:read, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// TestEmptyRoleDefaultsToGuest 验证空角色字符串也默认为 guest。
func TestEmptyRoleDefaultsToGuest(t *testing.T) {
	t.Parallel()

	enforcer, err := NewEnforcer("model.conf", "policy.csv")
	if err != nil {
		t.Fatalf("NewEnforcer() error = %v", err)
	}

	// guest 不能 config:read
	handler := enforcer.Middleware("config", "read")(testHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config", nil)
	// 显式设置空角色
	ctx := context.WithValue(req.Context(), contextKey{}, "")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for empty role on admin resource, got %d; body=%s", rec.Code, rec.Body.String())
	}
}
