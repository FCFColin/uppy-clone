//go:build integration

package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/handler"
	"github.com/uppy-clone/backend/internal/testsecrets"
)

func newAdminTokenString(jwtMgr *auth.JWTManager) string {
	now := time.Now()
	claims := jwt.MapClaims{
		"role": "admin",
		"sub":  "admin",
		"iat":  now.Unix(),
		"exp":  now.Add(30 * time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	signed, _ := token.SignedString(jwtMgr.PrivateKey())
	return signed
}

func TestAdminHandler_VerifyValidToken(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	adminHandler := handler.NewAdminHandler(nil, jwtMgr, nil)

	tokenStr := newAdminTokenString(jwtMgr)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	req.AddCookie(&http.Cookie{Name: "admin_token", Value: tokenStr})

	if !adminHandler.VerifyAdminToken(req) {
		t.Fatal("VerifyAdminToken should return true for valid admin token")
	}
}

func TestAdminHandler_VerifyNoCookie(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	adminHandler := handler.NewAdminHandler(nil, jwtMgr, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)

	if adminHandler.VerifyAdminToken(req) {
		t.Fatal("VerifyAdminToken should return false without cookie")
	}
}

func TestAdminHandler_VerifyInvalidToken(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	adminHandler := handler.NewAdminHandler(nil, jwtMgr, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	req.AddCookie(&http.Cookie{Name: "admin_token", Value: "invalid-token"})

	if adminHandler.VerifyAdminToken(req) {
		t.Fatal("VerifyAdminToken should return false for invalid token")
	}
}

func TestAdminHandler_VerifyWrongKey(t *testing.T) {
	mgr1 := auth.NewJWTManager("")
	mgr2 := auth.NewJWTManager("")

	tokenStr := newAdminTokenString(mgr1)

	adminHandler := handler.NewAdminHandler(nil, mgr2, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	req.AddCookie(&http.Cookie{Name: "admin_token", Value: tokenStr})

	if adminHandler.VerifyAdminToken(req) {
		t.Fatal("VerifyAdminToken should reject token signed with different key")
	}
}

func TestAdminHandler_VerifyNonAdminRole(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)

	now := time.Now()
	claims := jwt.MapClaims{
		"role": "user",
		"sub":  "admin",
		"iat":  now.Unix(),
		"exp":  now.Add(30 * time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tokenStr, err := token.SignedString(jwtMgr.PrivateKey())
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	adminHandler := handler.NewAdminHandler(nil, jwtMgr, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	req.AddCookie(&http.Cookie{Name: "admin_token", Value: tokenStr})

	if adminHandler.VerifyAdminToken(req) {
		t.Fatal("VerifyAdminToken should reject token with non-admin role")
	}
}

func TestAdminHandler_VerifyExpiredToken(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)

	now := time.Now()
	claims := jwt.MapClaims{
		"role": "admin",
		"sub":  "admin",
		"iat":  now.Add(-2 * time.Hour).Unix(),
		"exp":  now.Add(-1 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tokenStr, err := token.SignedString(jwtMgr.PrivateKey())
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	adminHandler := handler.NewAdminHandler(nil, jwtMgr, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	req.AddCookie(&http.Cookie{Name: "admin_token", Value: tokenStr})

	if adminHandler.VerifyAdminToken(req) {
		t.Fatal("VerifyAdminToken should reject expired token")
	}
}

func TestAdminHandler_VerifyNoneAlgorithm(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)

	token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"role": "admin",
		"sub":  "admin",
	})
	tokenStr, _ := token.SignedString(jwt.UnsafeAllowNoneSignatureType)

	adminHandler := handler.NewAdminHandler(nil, jwtMgr, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	req.AddCookie(&http.Cookie{Name: "admin_token", Value: tokenStr})

	if adminHandler.VerifyAdminToken(req) {
		t.Fatal("VerifyAdminToken should reject none-algorithm token")
	}
}
