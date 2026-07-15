package auth

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/requestctx"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/testsecrets"
)

type mockUserDataStore struct {
	user         *domain.User
	userErr      error
	results      []domain.GameResult
	resultsErr   error
	sessions     []domain.GameSession
	sessionsErr  error
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

func (m *mockUserDataStore) GetGameSessionsByUserID(_ context.Context, _ string) ([]domain.GameSession, error) {
	return m.sessions, m.sessionsErr
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
	// auth-013: ExportUserData now returns game results errors to the caller
	// instead of silently warning. GDPR compliance requires complete data.
	_, err := ExportUserData(context.Background(), store, "u1")
	if err == nil {
		t.Fatal("ExportUserData should fail on game results error")
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

func TestDeleteUserData_WithRefreshManager(t *testing.T) {
	t.Parallel()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	refreshMgr := NewRefreshTokenManager(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	ctx := context.Background()
	if _, err := refreshMgr.Generate(ctx, "u1"); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	store := &mockUserDataStore{}
	if err := DeleteUserData(ctx, nil, refreshMgr, nil, store, "u1", nil); err != nil {
		t.Fatalf("DeleteUserData: %v", err)
	}
}

func TestDeleteUserData_RevokesTokensFromRequest(t *testing.T) {
	t.Parallel()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	timeouts := config.DefaultTimeoutConfig()
	redisStore, err := store.NewRedisStore(mr.Addr(), timeouts)
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	defer func() { _ = redisStore.Close() }()

	jwtMgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	refreshMgr := NewRefreshTokenManager(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	ctx := context.Background()

	token, err := jwtMgr.SignToken("u1", "Nick")
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}
	if _, err := refreshMgr.Generate(ctx, "u1"); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/me", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})

	store := &mockUserDataStore{}
	if err := DeleteUserData(ctx, jwtMgr, refreshMgr, redisStore, store, "u1", req); err != nil {
		t.Fatalf("DeleteUserData: %v", err)
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

func (f *fakeRevocationChecker) IsJWTRevoked(_ context.Context, jti string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.revoked[jti], nil
}

// TestGetJTI_NoJTI verifies GetJTI returns empty string when no jti is in context.
func TestGetJTI_NoJTI(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if jti := GetJTI(req); jti != "" {
		t.Fatalf("GetJTI should return empty string; got %q", jti)
	}
}
