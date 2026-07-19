package middleware

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/testsecrets"
	"github.com/uppy-clone/backend/internal/util"
)

// fakeRevocationChecker is a test double for auth.JWTRevocationChecker.
type fakeRevocationChecker struct {
	revoked map[string]bool
	err     error
}

func newFakeRevocationChecker() *fakeRevocationChecker {
	return &fakeRevocationChecker{revoked: make(map[string]bool)}
}

func (f *fakeRevocationChecker) IsJWTRevoked(_ context.Context, jti string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.revoked[jti], nil
}

type redisRevoker struct {
	*fakeRevocationChecker
	client *redis.Client
}

func (r *redisRevoker) Client() *redis.Client { return r.client }

type scardFailHook struct{}

func (scardFailHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (scardFailHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() == "scard" {
			return errors.New("scard failed")
		}
		return next(ctx, cmd)
	}
}

func (scardFailHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

// --- detectMultiIPLogin tests ---

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
	rdb.AddHook(scardFailHook{})
	detectMultiIPLogin(context.Background(), rdb, "user-scard-err", "10.0.0.2")
}

// --- AuthMiddleware tests ---

func TestAuthMiddleware_RevokedTokenRejected(t *testing.T) {
	mgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	revoker := newFakeRevocationChecker()

	token, err := mgr.SignToken("user-123", "TestPlayer")
	if err != nil {
		t.Fatalf("SignToken failed: %v", err)
	}

	_, _, jti, _, err := mgr.VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken failed: %v", err)
	}

	revoker.revoked[jti] = true

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
	revoker := newFakeRevocationChecker()

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

func TestAuthMiddleware_RevokedSessionCookieRejected(t *testing.T) {
	mgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	revoker := newFakeRevocationChecker()

	token, _ := mgr.SignToken("user-session", "SessionPlayer")
	_, _, jti, _, _ := mgr.VerifyToken(token)
	revoker.revoked[jti] = true

	called := false
	handler := AuthMiddleware(mgr, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
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

func TestAuthMiddleware_InjectsRequestLogger(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	token, _ := jwtMgr.SignToken("user-log", "Logger")

	handler := AuthMiddleware(jwtMgr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if logger := util.LoggerFromContext(r.Context()); logger == nil {
			t.Fatal("expected logger in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(util.WithLogger(req.Context(), slog.New(slog.DiscardHandler)))
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAuthMiddleware_RevocationCheckError(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	revoker := &fakeRevocationChecker{err: context.DeadlineExceeded}
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

	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	token, _ := jwtMgr.SignToken("user-ip", "IPUser")

	called := false
	handler := AuthMiddleware(jwtMgr, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}), revoker)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), chiMiddleware.RequestIDKey, "req-1"))
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	req.RemoteAddr = "127.0.0.1:1234"
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()
	handler(rec, req)

	if !called || rec.Code != http.StatusOK {
		t.Fatalf("called=%v status=%d", called, rec.Code)
	}
}

func TestAuthMiddleware_UnauthorizedNoValidCookie(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	handler := AuthMiddleware(jwtMgr, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not run")
	}))

	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuthMiddleware_NoLoggerInContext(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	token, _ := jwtMgr.SignToken("user-1", "Nick")
	called := false
	handler := AuthMiddleware(jwtMgr, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()
	handler(rec, req)
	if !called || rec.Code != http.StatusOK {
		t.Fatalf("called=%v status=%d", called, rec.Code)
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
