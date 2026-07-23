package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/testsecrets"
	"github.com/uppy-clone/backend/internal/testutil"
)

// fakeRevocationChecker is now provided by internal/testutil (FakeRevocationChecker).

// --- AuthMiddleware tests ---

func TestAuthMiddleware_RevokedTokenRejected(t *testing.T) {
	mgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	revoker := testutil.NewFakeRevocationChecker()

	token, err := mgr.SignToken("user-123", "TestPlayer")
	if err != nil {
		t.Fatalf("SignToken failed: %v", err)
	}

	_, _, jti, _, err := mgr.VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken failed: %v", err)
	}

	revoker.Revoked[jti] = true

	called := false
	handler := AuthMiddleware(mgr, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

func TestAuthMiddleware_NonRevokedTokenAccepted(t *testing.T) {
	mgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	revoker := testutil.NewFakeRevocationChecker()

	token, err := mgr.SignToken("user-456", "AnotherPlayer")
	if err != nil {
		t.Fatalf("SignToken failed: %v", err)
	}

	called := false
	handler := AuthMiddleware(mgr, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

func TestAuthMiddleware_NoRevokerStillWorks(t *testing.T) {
	mgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)

	token, err := mgr.SignToken("user-789", "NoRevoker")
	if err != nil {
		t.Fatalf("SignToken failed: %v", err)
	}

	called := false
	handler := AuthMiddleware(mgr, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

func TestAuthMiddleware_JTIInContext(t *testing.T) {
	mgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)

	token, err := mgr.SignToken("user-jti", "JTIPlayer")
	if err != nil {
		t.Fatalf("SignToken failed: %v", err)
	}

	_, _, expectedJTI, _, _ := mgr.VerifyToken(token)

	var gotJTI string
	handler := AuthMiddleware(mgr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotJTI = auth.GetJTI(r)
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

func TestAuthMiddleware_InvalidCookieSkipped(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	validToken, _ := jwtMgr.SignToken("valid-user", "Valid")

	called := false
	handler := AuthMiddleware(jwtMgr, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "invalid.token.here"})
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: validToken})
	rec := httptest.NewRecorder()
	handler(rec, req)

	if !called || rec.Code != http.StatusOK {
		t.Fatalf("called=%v status=%d", called, rec.Code)
	}
}

func TestAuthMiddleware_RevocationCheckError(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	revoker := &testutil.FakeRevocationChecker{Err: context.DeadlineExceeded}
	token, _ := jwtMgr.SignToken("user-err", "Err")

	handler := AuthMiddleware(jwtMgr, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not run")
	}), revoker)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuthMiddleware_NoValidCookies(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	handler := AuthMiddleware(jwtMgr, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not run")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "not-a-jwt"})
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: "also-invalid"})
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// --- parseAuthCookie tests ---

// parseAuthCookie 是认证中间件的入口：从 Cookie 取 JWT 并交给 jwtMgr 验证。
// 失败路径必须返回错误，让上层中间件返回 401，而不是放行匿名请求。

// fakeTokenVerifier is a test double for auth.TokenVerifier.
type fakeTokenVerifier struct {
	verifyErr      error
	returnUserID   string
	returnNickname string
	returnJTI      string
	returnRole     string
	capturedToken  string
	callCount      int
}

func (f *fakeTokenVerifier) VerifyToken(tokenStr string) (userID, nickname, jti, role string, err error) {
	f.callCount++
	f.capturedToken = tokenStr
	if f.verifyErr != nil {
		return "", "", "", "", f.verifyErr
	}
	return f.returnUserID, f.returnNickname, f.returnJTI, f.returnRole, nil
}

func TestParseAuthCookie_Success(t *testing.T) {
	t.Parallel()

	const cookieName = "quickplay"
	const tokenValue = "fake.token.value"
	v := &fakeTokenVerifier{
		returnUserID:   "user-1",
		returnNickname: "Player1",
		returnJTI:      "jti-abc",
		returnRole:     "player",
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: tokenValue})

	uid, nick, jti, role, err := parseAuthCookie(req, cookieName, v)
	if err != nil {
		t.Fatalf("parseAuthCookie returned error: %v", err)
	}
	if uid != "user-1" {
		t.Errorf("userID = %q, want %q", uid, "user-1")
	}
	if nick != "Player1" {
		t.Errorf("nickname = %q, want %q", nick, "Player1")
	}
	if jti != "jti-abc" {
		t.Errorf("jti = %q, want %q", jti, "jti-abc")
	}
	if role != "player" {
		t.Errorf("role = %q, want %q", role, "player")
	}
	if v.capturedToken != tokenValue {
		t.Errorf("VerifyToken received %q, want %q", v.capturedToken, tokenValue)
	}
	if v.callCount != 1 {
		t.Errorf("VerifyToken call count = %d, want 1", v.callCount)
	}
}

func TestParseAuthCookie_MissingCookie(t *testing.T) {
	t.Parallel()

	v := &fakeTokenVerifier{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// No cookies added.

	uid, _, _, _, err := parseAuthCookie(req, "quickplay", v)
	if err == nil {
		t.Fatal("expected error for missing cookie, got nil")
	}
	if uid != "" {
		t.Errorf("userID = %q, want empty", uid)
	}
	if v.callCount != 0 {
		t.Errorf("VerifyToken should not be called when cookie missing; got callCount=%d", v.callCount)
	}
}

func TestParseAuthCookie_VerifyErrorPropagates(t *testing.T) {
	t.Parallel()

	verifyErr := errors.New("signature invalid")
	v := &fakeTokenVerifier{verifyErr: verifyErr}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "tampered.token"})

	_, _, _, _, err := parseAuthCookie(req, "session", v)
	if !errors.Is(err, verifyErr) {
		t.Fatalf("err = %v, want %v", err, verifyErr)
	}
}


