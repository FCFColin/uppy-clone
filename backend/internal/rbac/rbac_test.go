package rbac

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uppy-clone/backend/internal/auth"
)

func TestCheckPermission(t *testing.T) {
	e := NewEnforcer()
	tests := []struct {
		role, resource, action string
		want                   bool
	}{
		{RoleUser, "lobby", "create", true},
		{RoleGuest, "lobby", "create", false},
		{RoleGuest, "lobby", "read", true},
		{RoleAdmin, "users", "write", true},
		{RoleModerator, "users", "write", false},
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
	h := e.Middleware("user_data", "read")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	h := e.Middleware("lobby", "create")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req = req.WithContext(auth.WithRole(req.Context(), RoleUser))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !called || rec.Code != http.StatusOK {
		t.Fatalf("status=%d called=%v", rec.Code, called)
	}
}
