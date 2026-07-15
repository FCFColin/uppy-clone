package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/testsecrets"
)

func TestJWTManager_SignAndVerify(t *testing.T) {
	mgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)

	token, err := mgr.SignToken("user-1", "Player")
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}
	if _, _, _, _, err := mgr.VerifyToken(token); err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
}

func TestJWTManager_DifferentKeyFails(t *testing.T) {
	alien := NewJWTManager("")
	token, err := alien.SignToken("user-1", "Nick")
	if err != nil {
		t.Fatal(err)
	}
	mgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	if _, _, _, _, err := mgr.VerifyToken(token); err == nil {
		t.Fatal("expected verify failure when keys differ")
	}
}

func TestBuildRefreshCookie(t *testing.T) {
	cookie := BuildRefreshCookie("refresh-value", true)
	if cookie.Name != RefreshCookieName || cookie.Value != "refresh-value" || !cookie.Secure {
		t.Fatalf("refresh cookie = %+v", cookie)
	}

	cleared := BuildRefreshCookie("", true)
	if cleared.MaxAge != -1 {
		t.Fatalf("cleared refresh MaxAge = %d, want -1", cleared.MaxAge)
	}
}

func TestRefreshTokenFromRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := RefreshTokenFromRequest(req); got != "" {
		t.Fatalf("missing cookie should return empty, got %q", got)
	}

	req.AddCookie(&http.Cookie{Name: RefreshCookieName, Value: "rt-123"})
	if got := RefreshTokenFromRequest(req); got != "rt-123" {
		t.Fatalf("RefreshTokenFromRequest = %q", got)
	}
}

func TestBuildAuthCookie_Insecure(t *testing.T) {
	cookie := BuildAuthCookie("session", "token", config.CookieMaxAge, false)
	if cookie.Secure {
		t.Fatal("Secure should be false when isSecure=false")
	}
}
