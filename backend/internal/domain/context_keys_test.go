package domain

import (
	"context"
	"testing"
)

func TestWithRole_InjectsRole(t *testing.T) {
	t.Parallel()
	ctx := WithRole(context.Background(), RoleAdmin)
	role, ok := RoleFromContext(ctx)
	if !ok {
		t.Fatal("RoleFromContext should return ok=true after WithRole")
	}
	if role != RoleAdmin {
		t.Fatalf("role = %q, want %q", role, RoleAdmin)
	}
}

func TestRoleFromContext_EmptyContext(t *testing.T) {
	t.Parallel()
	role, ok := RoleFromContext(context.Background())
	if ok {
		t.Fatal("RoleFromContext should return ok=false on empty context")
	}
	if role != "" {
		t.Fatalf("role = %q, want empty", role)
	}
}

func TestWithRole_OverridesPreviousRole(t *testing.T) {
	t.Parallel()
	ctx := WithRole(context.Background(), RoleUser)
	ctx = WithRole(ctx, RoleAdmin)
	role, _ := RoleFromContext(ctx)
	if role != RoleAdmin {
		t.Fatalf("role = %q, want %q", role, RoleAdmin)
	}
}

func TestWithRole_PreservesOtherContextValues(t *testing.T) {
	t.Parallel()
	ctx := ContextKeyUserID.WithValue(context.Background(), "user-1")
	ctx = WithRole(ctx, RoleUser)

	userID, ok := ContextKeyUserID.Value(ctx)
	if !ok || userID != "user-1" {
		t.Fatalf("userID lost after WithRole: %q (ok=%v)", userID, ok)
	}
	role, ok := RoleFromContext(ctx)
	if !ok || role != RoleUser {
		t.Fatalf("role = %q (ok=%v)", role, ok)
	}
}

func TestRoleConstants(t *testing.T) {
	t.Parallel()
	if RoleUser != "user" {
		t.Errorf("RoleUser = %q, want %q", RoleUser, "user")
	}
	if RoleAdmin != "admin" {
		t.Errorf("RoleAdmin = %q, want %q", RoleAdmin, "admin")
	}
}

func TestContextKey_StringRepresentation(t *testing.T) {
	t.Parallel()
	// Verify the typed context key constants are stable string values.
	tests := []struct {
		key  ContextKey
		want string
	}{
		{ContextKeyUserID, "auth_user_id"},
		{ContextKeyNickname, "auth_nickname"},
		{ContextKeyRole, "auth_user_role"},
		{ContextKeyJTI, "auth_jti"},
	}
	for _, tt := range tests {
		if string(tt.key) != tt.want {
			t.Errorf("key = %q, want %q", string(tt.key), tt.want)
		}
	}
}
