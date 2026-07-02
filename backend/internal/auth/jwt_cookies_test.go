package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/testsecrets"
)

func TestNewJWTManagerWithRotation(t *testing.T) {
	primary := "primary-secret-key-padded-to-32-bytes!!"
	previous := "previous-secret-key-padded-to-32-bytes!"
	mgr := NewJWTManagerWithRotation(primary, previous)

	token, err := mgr.SignToken("user-1", "Player")
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}
	if _, _, _, err := mgr.VerifyToken(token); err != nil {
		t.Fatalf("VerifyToken with primary: %v", err)
	}

	legacyClaims := customClaims{
		Nickname: "Legacy",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-old",
			ID:        "legacy-jti",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	}
	legacyToken := jwt.NewWithClaims(jwt.SigningMethodHS256, legacyClaims)
	legacyString, err := legacyToken.SignedString([]byte(previous))
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	if _, _, _, err := mgr.VerifyToken(legacyString); err != nil {
		t.Fatalf("VerifyToken with previous secret: %v", err)
	}
}

func TestJWTManager_Secret(t *testing.T) {
	secret := testsecrets.TestJWTSecret
	mgr := NewJWTManager(secret)
	if got := string(mgr.Secret()); got != secret {
		t.Fatalf("Secret = %q, want %q", got, secret)
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

func TestVerifyToken_RotationBothKeysFail(t *testing.T) {
	mgr := NewJWTManagerWithRotation(
		"primary-secret-key-padded-to-32-bytes!!",
		"previous-secret-key-padded-to-32-bytes!",
	)
	alien := NewJWTManager("third-secret-key-padded-to-32-bytes!!")
	token, err := alien.SignToken("user-1", "Nick")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := mgr.VerifyToken(token); err == nil {
		t.Fatal("expected verify failure when neither primary nor previous key matches")
	}
}
