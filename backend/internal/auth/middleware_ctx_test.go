package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uppy-clone/backend/internal/testsecrets"
	"github.com/uppy-clone/backend/internal/testutil"
)

func TestGetAuthenticatedUser_FromContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(WithAuthenticatedUser(req.Context(), "user-1", "Nick"))

	uid, nick, ok := GetAuthenticatedUser(req)
	if !ok || uid != "user-1" || nick != "Nick" {
		t.Fatalf("GetAuthenticatedUser = (%q, %q, %v)", uid, nick, ok)
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
	revoker := testutil.NewFakeRevocationChecker()
	token, _ := jwtMgr.SignToken("user-rev", "Revoked")
	_, _, jti, _, _ := jwtMgr.VerifyToken(token)
	revoker.Revoked[jti] = true

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: token})

	if _, _, ok := AuthenticatedUserFromRequestWithRevocation(req, jwtMgr, revoker); ok {
		t.Fatal("revoked cookie should not authenticate")
	}
}

func TestAuthenticatedUserFromRequestWithRevocation_RevokerError(t *testing.T) {
	jwtMgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	revoker := &testutil.FakeRevocationChecker{Err: errors.New("redis down")}
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

func TestAuthenticatedUserFromRequestWithRevocation_RevokedContinues(t *testing.T) {
	jwtMgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	revoker := testutil.NewFakeRevocationChecker()
	revokedToken, _ := jwtMgr.SignToken("revoked-user", "Revoked")
	validToken, _ := jwtMgr.SignToken("valid-user", "Valid")
	_, _, revokedJTI, _, _ := jwtMgr.VerifyToken(revokedToken)
	revoker.Revoked[revokedJTI] = true

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
