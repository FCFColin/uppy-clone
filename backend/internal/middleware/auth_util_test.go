package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uppy-clone/backend/internal/auth"
)

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

func TestParseAuthCookie_EmptyCookieName(t *testing.T) {
	t.Parallel()

	v := &fakeTokenVerifier{
		returnUserID:   "user-x",
		returnNickname: "NickX",
	}
	// An empty cookie name never matches a real cookie, so r.Cookie returns
	// http.ErrNoCookie.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: "ignored"})

	_, _, _, _, err := parseAuthCookie(req, "", v)
	if err == nil {
		t.Fatal("expected error when cookie name is empty (no matching cookie)")
	}
	if v.callCount != 0 {
		t.Errorf("VerifyToken should not be called; callCount=%d", v.callCount)
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

func TestParseAuthCookie_JWTManagerSatisfiesTokenVerifier(t *testing.T) {
	t.Parallel()

	// Compile-time check that *auth.JWTManager satisfies auth.TokenVerifier.
	// This guards against an interface drift where the concrete JWT manager
	// loses its VerifyToken method or signature.
	var _ auth.TokenVerifier = (*auth.JWTManager)(nil)
}

func TestParseAuthCookie_LastCookieWins(t *testing.T) {
	t.Parallel()

	// Per RFC 6265, when a request carries two cookies with the same name,
	// r.Cookie returns the first one. Verify the parseAuthCookie honors that
	// behavior and does not concatenate or pick the wrong one.
	v := &fakeTokenVerifier{returnUserID: "user-first"}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: "first"})
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: "second"})

	uid, _, _, _, err := parseAuthCookie(req, "quickplay", v)
	if err != nil {
		t.Fatalf("parseAuthCookie returned error: %v", err)
	}
	if uid != "user-first" {
		t.Errorf("uid = %q, want %q", uid, "user-first")
	}
	if v.capturedToken != "first" {
		t.Errorf("VerifyToken received %q, want %q (first cookie wins)", v.capturedToken, "first")
	}
}
