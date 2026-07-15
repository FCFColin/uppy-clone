package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/testsecrets"
)

func TestGetAuthenticatedUser_FromContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(WithAuthenticatedUser(req.Context(), "user-1", "Nick"))

	uid, nick, ok := GetAuthenticatedUser(req)
	if !ok || uid != "user-1" || nick != "Nick" {
		t.Fatalf("GetAuthenticatedUser = (%q, %q, %v)", uid, nick, ok)
	}
}

func TestGetAuthenticatedUser_Missing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, _, ok := GetAuthenticatedUser(req); ok {
		t.Fatal("expected not ok without context")
	}
}

func TestAuthenticatedUserFromRequest_Cookie(t *testing.T) {
	jwtMgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	token, _ := jwtMgr.SignToken("cookie-user", "CookieNick")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})

	uid, nick, ok := AuthenticatedUserFromRequestWithRevocation(req, jwtMgr, nil)
	if !ok || uid != "cookie-user" || nick != "CookieNick" {
		t.Fatalf("AuthenticatedUserFromRequestWithRevocation = (%q, %q, %v)", uid, nick, ok)
	}
}

func TestAuthenticatedUserFromRequestWithRevocation_Revoked(t *testing.T) {
	jwtMgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	revoker := newFakeRevocationChecker()
	token, _ := jwtMgr.SignToken("user-rev", "Revoked")
	_, _, jti, _, _ := jwtMgr.VerifyToken(token)
	revoker.revoked[jti] = true

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: token})

	if _, _, ok := AuthenticatedUserFromRequestWithRevocation(req, jwtMgr, revoker); ok {
		t.Fatal("revoked cookie should not authenticate")
	}
}

func TestWithRoleAndRoleFromContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(domain.WithRole(req.Context(), "admin"))

	role, ok := domain.RoleFromContext(req.Context())
	if !ok || role != "admin" {
		t.Fatalf("RoleFromContext = (%q, %v)", role, ok)
	}
}

func TestAuthenticatedUserFromRequestWithRevocation_RevokerError(t *testing.T) {
	jwtMgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	revoker := &fakeRevocationChecker{err: errors.New("redis down")}
	token, _ := jwtMgr.SignToken("user-rev-err", "Nick")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})

	if _, _, ok := AuthenticatedUserFromRequestWithRevocation(req, jwtMgr, revoker); ok {
		t.Fatal("revoker error should reject authentication")
	}
}

func TestAuthenticatedUserFromRequest_NilJWTManager(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "any-token"})
	if _, _, ok := AuthenticatedUserFromRequestWithRevocation(req, nil, nil); ok {
		t.Fatal("nil jwt manager should not authenticate from cookie")
	}
}

func TestAuthenticatedUserFromCookies_RevokedSkipped(t *testing.T) {
	jwtMgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	revoker := newFakeRevocationChecker()
	token, _ := jwtMgr.SignToken("revoked-user", "Revoked")
	_, _, jti, _, _ := jwtMgr.VerifyToken(token)
	revoker.revoked[jti] = true

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	if _, _, ok := AuthenticatedUserFromRequestWithRevocation(req, jwtMgr, revoker); ok {
		t.Fatal("revoked cookie should not authenticate")
	}
}

func TestWithAuthenticatedUser(t *testing.T) {
	ctx := WithAuthenticatedUser(context.Background(), "u1", "n1")
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	if uid, nick, ok := GetAuthenticatedUser(req); !ok || uid != "u1" || nick != "n1" {
		t.Fatalf("GetAuthenticatedUser = (%q, %q, %v)", uid, nick, ok)
	}
}

func TestWithJTI(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(WithJTI(req.Context(), "jti-abc"))
	if got := GetJTI(req); got != "jti-abc" {
		t.Fatalf("GetJTI = %q", got)
	}
}

func TestAuthenticatedUserFromRequestWithRevocation_RevokedContinues(t *testing.T) {
	jwtMgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	revoker := newFakeRevocationChecker()
	revokedToken, _ := jwtMgr.SignToken("revoked-user", "Revoked")
	validToken, _ := jwtMgr.SignToken("valid-user", "Valid")
	_, _, revokedJTI, _, _ := jwtMgr.VerifyToken(revokedToken)
	revoker.revoked[revokedJTI] = true

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: revokedToken})
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: validToken})

	uid, nick, ok := AuthenticatedUserFromRequestWithRevocation(req, jwtMgr, revoker)
	if !ok || uid != "valid-user" || nick != "Valid" {
		t.Fatalf("AuthenticatedUserFromRequestWithRevocation = (%q, %q, %v)", uid, nick, ok)
	}
}

func TestAuthenticatedUserFromRequest_PrefersContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(WithAuthenticatedUser(req.Context(), "ctx-user", "CtxNick"))
	req.AddCookie(&http.Cookie{Name: "session", Value: "bad.token"})

	uid, nick, ok := AuthenticatedUserFromRequestWithRevocation(req, NewJWTManager(testsecrets.TestJWTPrivateKeyPEM), nil)
	if !ok || uid != "ctx-user" || nick != "CtxNick" {
		t.Fatalf("got (%q, %q, %v)", uid, nick, ok)
	}
}

func TestAuthenticatedUserFromRequest_NoAuth(t *testing.T) {
	jwtMgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	if _, _, ok := AuthenticatedUserFromRequestWithRevocation(httptest.NewRequest(http.MethodGet, "/", nil), jwtMgr, nil); ok {
		t.Fatal("expected false with no context and no valid cookies")
	}
}

func TestAuthenticatedUserFromCookies_FromContext(t *testing.T) {
	jwtMgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(WithAuthenticatedUser(req.Context(), "ctx-user", "CtxNick"))

	uid, nick, ok := authenticatedUserFromCookies(req, jwtMgr, nil)
	if !ok || uid != "ctx-user" || nick != "CtxNick" {
		t.Fatalf("authenticatedUserFromCookies = (%q, %q, %v)", uid, nick, ok)
	}
}
