package auth

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/requestctx"
)

type mockUserDataStore struct {
	user         *domain.User
	userErr      error
	results      []domain.GameResult
	resultsErr   error
	anonymizeErr error
}

func (m *mockUserDataStore) GetUserByID(_ context.Context, _ string) (*domain.User, error) {
	return m.user, m.userErr
}

func (m *mockUserDataStore) AnonymizeUser(_ context.Context, _ string) error {
	return m.anonymizeErr
}

func (m *mockUserDataStore) GetGameResultsByUserID(_ context.Context, _ string) ([]domain.GameResult, error) {
	return m.results, m.resultsErr
}

func TestExportUserData_Success(t *testing.T) {
	t.Parallel()
	store := &mockUserDataStore{
		user:    &domain.User{ID: "u1", Email: "test@example.com", Nickname: "Test"},
		results: []domain.GameResult{{ID: "r1", UserID: "u1"}},
	}
	data, err := ExportUserData(context.Background(), store, "u1")
	if err != nil {
		t.Fatalf("ExportUserData: %v", err)
	}
	if data["user"] == nil {
		t.Error("export should contain user")
	}
	if data["game_results"] == nil {
		t.Error("export should contain game_results")
	}
}

func TestExportUserData_UserNotFound(t *testing.T) {
	t.Parallel()
	store := &mockUserDataStore{user: nil, userErr: nil}
	_, err := ExportUserData(context.Background(), store, "nonexistent")
	if err == nil {
		t.Fatal("ExportUserData should return error for nil user")
	}
}

func TestExportUserData_StoreError(t *testing.T) {
	t.Parallel()
	store := &mockUserDataStore{userErr: errors.New("db down")}
	_, err := ExportUserData(context.Background(), store, "u1")
	if err == nil {
		t.Fatal("ExportUserData should return error when store fails")
	}
}

func TestExportUserData_NoGameResults(t *testing.T) {
	t.Parallel()
	store := &mockUserDataStore{
		user:    &domain.User{ID: "u1", Email: "a@b.com", Nickname: "A"},
		results: nil,
	}
	data, err := ExportUserData(context.Background(), store, "u1")
	if err != nil {
		t.Fatalf("ExportUserData: %v", err)
	}
	results, ok := data["game_results"]
	if !ok {
		t.Error("export should contain game_results key")
	}
	if results == nil {
		t.Error("game_results should not be nil even when empty")
	}
}

func TestExportUserData_GameResultsError(t *testing.T) {
	t.Parallel()
	store := &mockUserDataStore{
		user:       &domain.User{ID: "u1", Email: "a@b.com", Nickname: "A"},
		resultsErr: errors.New("query failed"),
	}
	data, err := ExportUserData(context.Background(), store, "u1")
	if err != nil {
		t.Fatalf("ExportUserData should not fail on game results error: %v", err)
	}
	if data["game_results"] == nil {
		t.Error("export should have empty game_results on error")
	}
}

func TestDeleteUserData_NilDataStore(t *testing.T) {
	t.Parallel()
	err := DeleteUserData(context.Background(), nil, nil, nil, nil, "u1", nil)
	if err != nil {
		t.Errorf("DeleteUserData with nil dataStore should succeed: %v", err)
	}
}

func TestDeleteUserData_AnonymizeError(t *testing.T) {
	t.Parallel()
	store := &mockUserDataStore{anonymizeErr: errors.New("anonymize failed")}
	err := DeleteUserData(context.Background(), nil, nil, nil, store, "u1", nil)
	if err == nil {
		t.Fatal("DeleteUserData should return error when anonymize fails")
	}
}

func TestDeleteUserData_AnonymizeSuccess(t *testing.T) {
	t.Parallel()
	store := &mockUserDataStore{}
	err := DeleteUserData(context.Background(), nil, nil, nil, store, "u1", nil)
	if err != nil {
		t.Errorf("DeleteUserData should succeed: %v", err)
	}
}

func TestIsSecure(t *testing.T) {
	tests := []struct {
		name     string
		setupReq func() *http.Request
		want     bool
	}{
		{
			name: "direct HTTPS (r.TLS != nil) returns true",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
				req.TLS = &tls.ConnectionState{}
				return req
			},
			want: true,
		},
		{
			name: "untrusted X-Forwarded-Proto: https returns false",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
				req.Header.Set("X-Forwarded-Proto", "https")
				return req
			},
			want: false,
		},
		{
			name: "trusted X-Forwarded-Proto: https returns true",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
				req = req.WithContext(requestctx.WithTrustedProxy(req.Context(), true))
				req.Header.Set("X-Forwarded-Proto", "https")
				return req
			},
			want: true,
		},
		{
			name: "trusted X-Forwarded-Proto: http returns false",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
				req = req.WithContext(requestctx.WithTrustedProxy(req.Context(), true))
				req.Header.Set("X-Forwarded-Proto", "http")
				return req
			},
			want: false,
		},
		{
			name: "no TLS, no header returns false",
			setupReq: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSecure(tt.setupReq())
			if got != tt.want {
				t.Errorf("IsSecure() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGameEndedOutboxPayload(t *testing.T) {
	t.Parallel()
	payload := map[string]interface{}{
		"session_id": "sess-123",
		"score":      100,
	}
	data, err := GameEndedOutboxPayload(payload)
	if err != nil {
		t.Fatalf("GameEndedOutboxPayload: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["event"] != "game.ended" {
		t.Errorf("event = %v, want game.ended", result["event"])
	}
}

func TestGameEndedOutboxPayload_NilPayload(t *testing.T) {
	t.Parallel()
	data, err := GameEndedOutboxPayload(nil)
	if err != nil {
		t.Fatalf("GameEndedOutboxPayload(nil): %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["data"] != nil {
		t.Errorf("data should be nil, got %v", result["data"])
	}
}

func TestGameEndedOutboxPayload_EmptyPayload(t *testing.T) {
	t.Parallel()
	data, err := GameEndedOutboxPayload(map[string]interface{}{})
	if err != nil {
		t.Fatalf("GameEndedOutboxPayload(empty): %v", err)
	}
	if len(data) == 0 {
		t.Error("result should not be empty")
	}
}

// fakeRevocationChecker is a test double for JWTRevocationChecker.
type fakeRevocationChecker struct {
	revoked map[string]bool
	err     error
}

func newFakeRevocationChecker() *fakeRevocationChecker {
	return &fakeRevocationChecker{
		revoked: make(map[string]bool),
	}
}

func (f *fakeRevocationChecker) IsJWTRevoked(ctx context.Context, jti string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.revoked[jti], nil
}

// TestAuthMiddleware_RevokedTokenRejected verifies that a revoked JWT
// (jti in revocation list) is rejected with 401.
func TestAuthMiddleware_RevokedTokenRejected(t *testing.T) {
	mgr := NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	revoker := newFakeRevocationChecker()

	token, err := mgr.SignToken("user-123", "TestPlayer")
	if err != nil {
		t.Fatalf("SignToken failed: %v", err)
	}

	// Extract jti from the token
	_, _, jti, err := mgr.VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken failed: %v", err)
	}

	// Revoke the token's jti
	revoker.revoked[jti] = true

	called := false
	handler := AuthMiddleware(mgr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

// TestAuthMiddleware_NonRevokedTokenAccepted verifies that a non-revoked JWT
// is accepted and the handler is called.
func TestAuthMiddleware_NonRevokedTokenAccepted(t *testing.T) {
	mgr := NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	revoker := newFakeRevocationChecker()

	token, err := mgr.SignToken("user-456", "AnotherPlayer")
	if err != nil {
		t.Fatalf("SignToken failed: %v", err)
	}

	called := false
	handler := AuthMiddleware(mgr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

// TestAuthMiddleware_NoRevokerStillWorks verifies that when no revoker is
// provided, the middleware works as before (backward compatible).
func TestAuthMiddleware_NoRevokerStillWorks(t *testing.T) {
	mgr := NewJWTManager("test-secret-key-0123456789abcdef0123456789")

	token, err := mgr.SignToken("user-789", "NoRevoker")
	if err != nil {
		t.Fatalf("SignToken failed: %v", err)
	}

	called := false
	handler := AuthMiddleware(mgr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

// TestAuthMiddleware_JTIInContext verifies that the jti is available in
// the request context after authentication.
func TestAuthMiddleware_JTIInContext(t *testing.T) {
	mgr := NewJWTManager("test-secret-key-0123456789abcdef0123456789")

	token, err := mgr.SignToken("user-jti", "JTIPlayer")
	if err != nil {
		t.Fatalf("SignToken failed: %v", err)
	}

	_, _, expectedJTI, _ := mgr.VerifyToken(token)

	var gotJTI string
	handler := AuthMiddleware(mgr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotJTI = GetJTI(r)
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

// TestAuthMiddleware_RevokedSessionCookieRejected verifies that a revoked
// session cookie (not quickplay) is also rejected.
func TestAuthMiddleware_RevokedSessionCookieRejected(t *testing.T) {
	mgr := NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	revoker := newFakeRevocationChecker()

	token, _ := mgr.SignToken("user-session", "SessionPlayer")
	_, _, jti, _ := mgr.VerifyToken(token)
	revoker.revoked[jti] = true

	called := false
	handler := AuthMiddleware(mgr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

// TestGetJTI_NoJTI verifies GetJTI returns empty string when no jti is in context.
func TestGetJTI_NoJTI(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if jti := GetJTI(req); jti != "" {
		t.Fatalf("GetJTI should return empty string; got %q", jti)
	}
}
