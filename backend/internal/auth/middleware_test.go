package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeRevocationChecker is a test double for JWTRevocationChecker.
type fakeRevocationChecker struct {
	revoked map[string]bool
	err     error
}

func newFakeRevocationChecker() *fakeRevocationChecker {
	return &fakeRevocationChecker{
		revoked: make(map[string]bool),
	}
}

func (f *fakeRevocationChecker) IsJWTRevoked(ctx context.Context, jti string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.revoked[jti], nil
}

// TestAuthMiddleware_RevokedTokenRejected verifies that a revoked JWT
// (jti in revocation list) is rejected with 401.
func TestAuthMiddleware_RevokedTokenRejected(t *testing.T) {
	mgr := NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	revoker := newFakeRevocationChecker()

	token, err := mgr.SignToken("user-123", "TestPlayer")
	if err != nil {
		t.Fatalf("SignToken failed: %v", err)
	}

	// Extract jti from the token
	_, _, jti, err := mgr.VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken failed: %v", err)
	}

	// Revoke the token's jti
	revoker.revoked[jti] = true

	called := false
	handler := AuthMiddleware(mgr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}), revoker)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: token})
	rec := httptest.NewRecorder()

	handler(rec, req)

	if called {
		t.Fatal("handler should NOT be called for revoked token")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want %d", rec.Code, http.StatusUnauthorized)
	}
}

// TestAuthMiddleware_NonRevokedTokenAccepted verifies that a non-revoked JWT
// is accepted and the handler is called.
func TestAuthMiddleware_NonRevokedTokenAccepted(t *testing.T) {
	mgr := NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	revoker := newFakeRevocationChecker()

	token, err := mgr.SignToken("user-456", "AnotherPlayer")
	if err != nil {
		t.Fatalf("SignToken failed: %v", err)
	}

	called := false
	handler := AuthMiddleware(mgr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}), revoker)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: token})
	rec := httptest.NewRecorder()

	handler(rec, req)

	if !called {
		t.Fatal("handler should be called for non-revoked token")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want %d", rec.Code, http.StatusOK)
	}
}

// TestAuthMiddleware_NoRevokerStillWorks verifies that when no revoker is
// provided, the middleware works as before (backward compatible).
func TestAuthMiddleware_NoRevokerStillWorks(t *testing.T) {
	mgr := NewJWTManager("test-secret-key-0123456789abcdef0123456789")

	token, err := mgr.SignToken("user-789", "NoRevoker")
	if err != nil {
		t.Fatalf("SignToken failed: %v", err)
	}

	called := false
	handler := AuthMiddleware(mgr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()

	handler(rec, req)

	if !called {
		t.Fatal("handler should be called when no revoker is provided")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want %d", rec.Code, http.StatusOK)
	}
}

// TestAuthMiddleware_JTIInContext verifies that the jti is available in
// the request context after authentication.
func TestAuthMiddleware_JTIInContext(t *testing.T) {
	mgr := NewJWTManager("test-secret-key-0123456789abcdef0123456789")

	token, err := mgr.SignToken("user-jti", "JTIPlayer")
	if err != nil {
		t.Fatalf("SignToken failed: %v", err)
	}

	_, _, expectedJTI, _ := mgr.VerifyToken(token)

	var gotJTI string
	handler := AuthMiddleware(mgr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotJTI = GetJTI(r)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: token})
	rec := httptest.NewRecorder()

	handler(rec, req)

	if gotJTI != expectedJTI {
		t.Fatalf("jti in context = %q; want %q", gotJTI, expectedJTI)
	}
}

// TestAuthMiddleware_RevokedSessionCookieRejected verifies that a revoked
// session cookie (not quickplay) is also rejected.
func TestAuthMiddleware_RevokedSessionCookieRejected(t *testing.T) {
	mgr := NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	revoker := newFakeRevocationChecker()

	token, _ := mgr.SignToken("user-session", "SessionPlayer")
	_, _, jti, _ := mgr.VerifyToken(token)
	revoker.revoked[jti] = true

	called := false
	handler := AuthMiddleware(mgr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}), revoker)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()

	handler(rec, req)

	if called {
		t.Fatal("handler should NOT be called for revoked session cookie")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want %d", rec.Code, http.StatusUnauthorized)
	}
}

// TestGetJTI_NoJTI verifies GetJTI returns empty string when no jti is in context.
func TestGetJTI_NoJTI(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if jti := GetJTI(req); jti != "" {
		t.Fatalf("GetJTI should return empty string; got %q", jti)
	}
}
