package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"

	"github.com/uppy-clone/backend/internal/slogctx"
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
	jwtMgr := NewJWTManager("test-secret-key-padded-to-32-bytes!!")
	token, _ := jwtMgr.SignToken("cookie-user", "CookieNick")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})

	uid, nick, ok := AuthenticatedUserFromRequest(req, jwtMgr)
	if !ok || uid != "cookie-user" || nick != "CookieNick" {
		t.Fatalf("AuthenticatedUserFromRequest = (%q, %q, %v)", uid, nick, ok)
	}
}

func TestAuthenticatedUserFromRequestWithRevocation_Revoked(t *testing.T) {
	jwtMgr := NewJWTManager("test-secret-key-padded-to-32-bytes!!")
	revoker := newFakeRevocationChecker()
	token, _ := jwtMgr.SignToken("user-rev", "Revoked")
	_, _, jti, _ := jwtMgr.VerifyToken(token)
	revoker.revoked[jti] = true

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: token})

	if _, _, ok := AuthenticatedUserFromRequestWithRevocation(req, jwtMgr, revoker); ok {
		t.Fatal("revoked cookie should not authenticate")
	}
}

func TestWithRoleAndRoleFromContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(WithRole(req.Context(), "admin"))

	role, ok := RoleFromContext(req)
	if !ok || role != "admin" {
		t.Fatalf("RoleFromContext = (%q, %v)", role, ok)
	}
}

func TestDetectMultiIPLogin_AlertThreshold(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()

	for i := 0; i < maxIPsPerHour+1; i++ {
		detectMultiIPLogin(ctx, rdb, "user-multi", fmt.Sprintf("10.0.0.%d", i+1))
	}
}

func TestDetectMultiIPLogin_EarlyReturns(t *testing.T) {
	ctx := context.Background()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	detectMultiIPLogin(ctx, nil, "user", "1.2.3.4")
	detectMultiIPLogin(ctx, rdb, "", "1.2.3.4")
	detectMultiIPLogin(ctx, rdb, "user", "")
}

func TestDetectMultiIPLogin_SAddError(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	mr.SetError("redis unavailable")

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	detectMultiIPLogin(context.Background(), rdb, "user-sadd-err", "10.0.0.1")
}

func TestDetectMultiIPLogin_SCardError(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()
	ipKey := "user:ips:user-scard-err"
	if err := rdb.SAdd(ctx, ipKey, "10.0.0.1").Err(); err != nil {
		t.Fatalf("SAdd: %v", err)
	}
	mr.SetError("redis unavailable")
	detectMultiIPLogin(ctx, rdb, "user-scard-err", "10.0.0.2")
}

func TestAuthenticatedUserFromRequestWithRevocation_RevokerError(t *testing.T) {
	jwtMgr := NewJWTManager("test-secret-key-padded-to-32-bytes!!")
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
	if _, _, ok := AuthenticatedUserFromRequest(req, nil); ok {
		t.Fatal("nil jwt manager should not authenticate from cookie")
	}
}

func TestAuthMiddleware_InvalidCookieSkipped(t *testing.T) {
	jwtMgr := NewJWTManager("test-secret-key-padded-to-32-bytes!!")
	validToken, _ := jwtMgr.SignToken("valid-user", "Valid")

	called := false
	handler := AuthMiddleware(jwtMgr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestAuthMiddleware_InjectsRequestLogger(t *testing.T) {
	jwtMgr := NewJWTManager("test-secret-key-padded-to-32-bytes!!")
	token, _ := jwtMgr.SignToken("user-log", "Logger")

	handler := AuthMiddleware(jwtMgr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if logger := slogctx.LoggerFromContext(r.Context()); logger == nil {
			t.Fatal("expected logger in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(slogctx.WithLogger(req.Context(), slog.New(slog.DiscardHandler)))
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAuthMiddleware_RevocationCheckError(t *testing.T) {
	jwtMgr := NewJWTManager("test-secret-key-padded-to-32-bytes!!")
	revoker := &fakeRevocationChecker{err: context.DeadlineExceeded}
	token, _ := jwtMgr.SignToken("user-err", "Err")

	handler := AuthMiddleware(jwtMgr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

type redisRevoker struct {
	*fakeRevocationChecker
	client *redis.Client
}

func (r *redisRevoker) Client() *redis.Client { return r.client }

func TestAuthenticatedUserFromRequestWithRevocation_RevokedContinues(t *testing.T) {
	jwtMgr := NewJWTManager("test-secret-key-padded-to-32-bytes!!")
	revoker := newFakeRevocationChecker()
	revokedToken, _ := jwtMgr.SignToken("revoked-user", "Revoked")
	validToken, _ := jwtMgr.SignToken("valid-user", "Valid")
	_, _, revokedJTI, _ := jwtMgr.VerifyToken(revokedToken)
	revoker.revoked[revokedJTI] = true

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: revokedToken})
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: validToken})

	uid, nick, ok := AuthenticatedUserFromRequestWithRevocation(req, jwtMgr, revoker)
	if !ok || uid != "valid-user" || nick != "Valid" {
		t.Fatalf("AuthenticatedUserFromRequestWithRevocation = (%q, %q, %v)", uid, nick, ok)
	}
}

func TestAuthMiddleware_MultiIPWithRedisProvider(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	revoker := &redisRevoker{
		fakeRevocationChecker: newFakeRevocationChecker(),
		client:                redis.NewClient(&redis.Options{Addr: mr.Addr()}),
	}

	jwtMgr := NewJWTManager("test-secret-key-padded-to-32-bytes!!")
	token, _ := jwtMgr.SignToken("user-ip", "IPUser")

	called := false
	handler := AuthMiddleware(jwtMgr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}), revoker)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.RequestIDKey, "req-1"))
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	req.RemoteAddr = "127.0.0.1:1234"
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()
	handler(rec, req)

	if !called || rec.Code != http.StatusOK {
		t.Fatalf("called=%v status=%d", called, rec.Code)
	}
}
