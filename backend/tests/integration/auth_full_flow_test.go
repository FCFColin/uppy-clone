//go:build integration

package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/testsecrets"
	"github.com/uppy-clone/backend/internal/testutil"
)

func TestAuth_JWTSignAndVerify(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)

	token, err := jwtMgr.SignToken("user-1", "PlayerOne")
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	userID, nickname, jti, err := jwtMgr.VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if userID != "user-1" {
		t.Fatalf("userID = %q, want user-1", userID)
	}
	if nickname != "PlayerOne" {
		t.Fatalf("nickname = %q, want PlayerOne", nickname)
	}
	if jti == "" {
		t.Fatal("expected non-empty jti")
	}
}

func TestAuth_JWTTamperedTokenRejected(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)

	token, err := jwtMgr.SignToken("user-tamper", "Tamper")
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}

	tampered := token[:len(token)-5] + "XXXXX"

	_, _, _, err = jwtMgr.VerifyToken(tampered)
	if err == nil {
		t.Fatal("expected error for tampered token")
	}
}

func TestAuth_JWTInvalidTokenRejected(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)

	_, _, _, err := jwtMgr.VerifyToken("invalid-token-string")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestAuth_JWTWrongKey(t *testing.T) {
	mgr1 := auth.NewJWTManager("")
	mgr2 := auth.NewJWTManager("")

	token, err := mgr1.SignToken("user-1", "Player")
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}

	_, _, _, err = mgr2.VerifyToken(token)
	if err == nil {
		t.Fatal("expected verification failure with different key")
	}

	_, _, _, err = mgr1.VerifyToken(token)
	if err != nil {
		t.Fatalf("own key should verify: %v", err)
	}
}

func TestAuth_RevokeAndCheckJWT(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	ctx := context.Background()

	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	token, err := jwtMgr.SignToken("user-revoke", "Revocable")
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}

	_, _, jti, err := jwtMgr.VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}

	revoked, err := redisStore.IsJWTRevoked(ctx, jti)
	if err != nil {
		t.Fatalf("IsJWTRevoked: %v", err)
	}
	if revoked {
		t.Fatal("expected token to not be revoked initially")
	}

	if err := redisStore.RevokeJWT(ctx, jti, config.AccessTokenTTL); err != nil {
		t.Fatalf("RevokeJWT: %v", err)
	}

	revoked, err = redisStore.IsJWTRevoked(ctx, jti)
	if err != nil {
		t.Fatalf("IsJWTRevoked after revoke: %v", err)
	}
	if !revoked {
		t.Fatal("expected token to be revoked")
	}
}

func TestAuth_RefreshTokenFlow(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	ctx := context.Background()

	refreshMgr := auth.NewRefreshTokenManager(redisStore.Client())

	oldToken, err := refreshMgr.Generate(ctx, "user-refresh")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if oldToken == "" {
		t.Fatal("expected non-empty refresh token")
	}

	result, err := refreshMgr.ConsumeRefreshToken(ctx, oldToken)
	if err != nil {
		t.Fatalf("ConsumeRefreshToken: %v", err)
	}
	if result.Reused {
		t.Fatal("expected no reuse on first consume")
	}
	if result.UserID != "user-refresh" {
		t.Fatalf("UserID = %q, want user-refresh", result.UserID)
	}

	_, err = refreshMgr.Validate(ctx, oldToken)
	if err == nil {
		t.Fatal("expected old refresh token to be consumed")
	}

	result2, err := refreshMgr.ConsumeRefreshToken(ctx, oldToken)
	if err != nil {
		t.Fatalf("ConsumeRefreshToken second: %v", err)
	}
	if !result2.Reused {
		t.Fatal("expected reuse flag on second consume")
	}
}

func TestAuth_RefreshTokenInvalid(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	ctx := context.Background()

	refreshMgr := auth.NewRefreshTokenManager(redisStore.Client())

	_, err := refreshMgr.ConsumeRefreshToken(ctx, "invalid-token")
	if err == nil {
		t.Fatal("expected error for invalid refresh token")
	}
}

func TestAuth_RefreshTokenRevoke(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	ctx := context.Background()

	refreshMgr := auth.NewRefreshTokenManager(redisStore.Client())

	token, err := refreshMgr.Generate(ctx, "user-revoke-test")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if err := refreshMgr.Revoke(ctx, token); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	_, err = refreshMgr.Validate(ctx, token)
	if err == nil {
		t.Fatal("expected revoked token to be invalid")
	}
}

func TestAuth_QuickPlayWithRealDB(t *testing.T) {
	db := testutil.SetupPostgresStore(t)
	redisStore := testutil.SetupMiniredisStore(t)

	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	refreshMgr := auth.NewRefreshTokenManager(redisStore.Client())

	req := httptest.NewRequest(http.MethodPost, "https://example.com/quickplay", strings.NewReader(`{"nickname":"IntegrationPlayer"}`))

	cookie, resp, err := auth.QuickPlay(db, jwtMgr, refreshMgr, nil, "IntegrationPlayer", req)
	if err != nil {
		t.Fatalf("QuickPlay: %v", err)
	}
	if cookie == nil {
		t.Fatal("expected non-nil cookie")
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.UserID == "" {
		t.Fatal("expected non-empty UserID")
	}
	if resp.Nickname == "" {
		t.Fatal("expected non-empty Nickname")
	}
}

func TestAuth_QuickPlayExistingSession(t *testing.T) {
	db := testutil.SetupPostgresStore(t)
	redisStore := testutil.SetupMiniredisStore(t)

	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	refreshMgr := auth.NewRefreshTokenManager(redisStore.Client())

	firstReq := httptest.NewRequest(http.MethodPost, "https://example.com/quickplay", strings.NewReader(`{"nickname":"First"}`))
	_, firstResp, err := auth.QuickPlay(db, jwtMgr, refreshMgr, nil, "First", firstReq)
	if err != nil {
		t.Fatalf("first QuickPlay: %v", err)
	}

	token, err := jwtMgr.SignToken(firstResp.UserID, firstResp.Nickname)
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "https://example.com/quickplay", strings.NewReader(`{"nickname":"Second"}`))
	secondReq.AddCookie(&http.Cookie{Name: "quickplay", Value: token})

	_, secondResp, err := auth.QuickPlay(db, jwtMgr, refreshMgr, nil, "Second", secondReq)
	if err != nil {
		t.Fatalf("second QuickPlay: %v", err)
	}
	if secondResp.UserID != firstResp.UserID {
		t.Fatalf("UserID = %q, want %q (same user on existing session)", secondResp.UserID, firstResp.UserID)
	}
}

func TestAuth_RevokeAllForUser(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	ctx := context.Background()

	refreshMgr := auth.NewRefreshTokenManager(redisStore.Client())

	token1, err := refreshMgr.Generate(ctx, "user-all-revoke")
	if err != nil {
		t.Fatalf("Generate 1: %v", err)
	}
	token2, err := refreshMgr.Generate(ctx, "user-all-revoke")
	if err != nil {
		t.Fatalf("Generate 2: %v", err)
	}

	if err := refreshMgr.RevokeAllForUser(ctx, "user-all-revoke"); err != nil {
		t.Fatalf("RevokeAllForUser: %v", err)
	}

	_, err = refreshMgr.Validate(ctx, token1)
	if err == nil {
		t.Fatal("expected token1 to be revoked")
	}
	_, err = refreshMgr.Validate(ctx, token2)
	if err == nil {
		t.Fatal("expected token2 to be revoked")
	}
}
