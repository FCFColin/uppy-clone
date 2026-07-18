package rbac

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uppy-clone/backend/internal/domain"
)

func TestCheckPermission(t *testing.T) {
	e := NewEnforcer()
	tests := []struct {
		role, resource, action string
		want                   bool
	}{
		{domain.RoleUser, "lobby", "create", true},
		{RoleGuest, "lobby", "create", false},
		{RoleGuest, "lobby", "read", true},
		{domain.RoleAdmin, "users", "write", true},
		{"unknown", "lobby", "read", false},
		{domain.RoleUser, "unknown_resource", "read", false},
		{domain.RoleAdmin, "lobby", "delete", false},
	}
	for _, tc := range tests {
		if got := e.CheckPermission(tc.role, tc.resource, tc.action); got != tc.want {
			t.Errorf("%s/%s/%s = %v, want %v", tc.role, tc.resource, tc.action, got, tc.want)
		}
	}
}

func TestMiddleware_DeniesGuestForUserData(t *testing.T) {
	e := NewEnforcer()
	called := false
	h := e.Middleware("user_data", "read")(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || called {
		t.Fatalf("status=%d called=%v", rec.Code, called)
	}
}

func TestMiddleware_AllowsAuthenticatedUser(t *testing.T) {
	e := NewEnforcer()
	called := false
	h := e.Middleware("lobby", "create")(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req = req.WithContext(domain.WithRole(req.Context(), domain.RoleUser))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !called || rec.Code != http.StatusOK {
		t.Fatalf("status=%d called=%v", rec.Code, called)
	}
}
