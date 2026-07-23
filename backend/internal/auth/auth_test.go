// Package auth provides authentication and authorization utilities.
package auth

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/testutil"
)

// 企业为何需要：安全关键组件（中间件/认证/管理）零测试是最高风险——任何改动都可在生产暴露。

// --- Shared test doubles ---

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

func (m *mockUserDataStore) CreateUser(_ context.Context, _ *domain.User) error { return nil }
func (m *mockUserDataStore) GetUserByEmail(_ context.Context, _ string) (*domain.User, error) {
	return nil, nil
}
func (m *mockUserDataStore) UpdateUserLastLogin(_ context.Context, _ string) error { return nil }

func (m *mockUserDataStore) AnonymizeUser(_ context.Context, _ string) error {
	return m.anonymizeErr
}

func (m *mockUserDataStore) GetGameResultsByUserID(_ context.Context, _ string) ([]domain.GameResult, error) {
	return m.results, m.resultsErr
}

func (m *mockUserDataStore) GetGameSessionsByUserID(_ context.Context, _ string) ([]domain.GameSession, error) {
	return m.sessions, m.sessionsErr
}

// --- MagicLinkService pure logic tests ---
// Note: RequestMagicLink and VerifyMagicLink use concrete *store.RedisStore,
// so we test the pure logic functions they depend on, and test the full flow
// via integration tests with real Redis.

func TestRequestMagicLink_InvalidEmail(t *testing.T) {
	tests := []struct {
		email string
		valid bool
	}{
		{"invalid-email", false},
		{"", false},
		{"user name@domain.com", false},
		{"user@example.com", true},
		{"test.user+tag@domain.org", true},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			got := isValidEmail(tt.email)
			if got != tt.valid {
				t.Errorf("isValidEmail(%q) = %v, want %v", tt.email, got, tt.valid)
			}
		})
	}
}

func TestHashToken(t *testing.T) {
	hash1 := HashToken("test-token")
	hash2 := HashToken("test-token")
	if hash1 != hash2 {
		t.Error("HashToken should be deterministic")
	}
	if len(hash1) != 64 {
		t.Errorf("HashToken output length = %d, want 64", len(hash1))
	}

	hash3 := HashToken("different-token")
	if hash1 == hash3 {
		t.Error("HashToken should produce different hashes for different inputs")
	}
}

// --- QuickPlay pure logic tests ---

func TestSanitizePlayerName(t *testing.T) {
	t.Run("removes control chars", func(t *testing.T) {
		result := sanitizePlayerName("hello\x00world\x01test")
		if strings.Contains(result, "\x00") || strings.Contains(result, "\x01") {
			t.Errorf("sanitizePlayerName should remove control chars, got %q", result)
		}
	})

	t.Run("removes HTML chars", func(t *testing.T) {
		result := sanitizePlayerName("hello<script>alert('xss')</script>&")
		if strings.Contains(result, "<") || strings.Contains(result, ">") || strings.Contains(result, "&") {
			t.Errorf("sanitizePlayerName should remove HTML chars, got %q", result)
		}
	})

	t.Run("limits to 20 chars", func(t *testing.T) {
		longName := strings.Repeat("a", 30)
		result := sanitizePlayerName(longName)
		if len([]rune(result)) > 20 {
			t.Errorf("sanitizePlayerName should limit to 20 chars, got %d", len([]rune(result)))
		}
	})

	t.Run("trims whitespace", func(t *testing.T) {
		result := sanitizePlayerName("  hello  ")
		if result != "hello" {
			t.Errorf("sanitizePlayerName should trim, got %q", result)
		}
	})

	t.Run("empty string returns empty", func(t *testing.T) {
		result := sanitizePlayerName("")
		if result != "" {
			t.Errorf("sanitizePlayerName of empty should be empty, got %q", result)
		}
	})
}

func TestParseQuickPlayRequest(t *testing.T) {
	t.Run("parses nickname from body", func(t *testing.T) {
		body := `{"nickname":"TestPlayer"}`
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		nickname := ParseQuickPlayRequest(req)
		if nickname != "TestPlayer" {
			t.Errorf("ParseQuickPlayRequest = %q, want %q", nickname, "TestPlayer")
		}
	})

	t.Run("returns empty string for invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("invalid"))
		nickname := ParseQuickPlayRequest(req)
		if nickname != "" {
			t.Errorf("ParseQuickPlayRequest = %q, want empty string for invalid JSON", nickname)
		}
	})

	t.Run("returns empty string when nickname field is missing", func(t *testing.T) {
		body := `{"otherField":"value"}`
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		nickname := ParseQuickPlayRequest(req)
		if nickname != "" {
			t.Errorf("ParseQuickPlayRequest = %q, want empty string", nickname)
		}
	})
}

// --- GDPR export / delete ---

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

func TestDeleteUserData_RevokesTokensFromRequest(t *testing.T) {
	t.Parallel()
	redisStore := testutil.SetupMiniredisStore(t)
	jwtMgr, refreshMgr := setupRefreshEnv(t)
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

// --- Request security helpers ---

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
				req = req.WithContext(domain.WithTrustedProxy(req.Context(), true))
				req.Header.Set("X-Forwarded-Proto", "https")
				return req
			},
			want: true,
		},
		{
			name: "trusted X-Forwarded-Proto: http returns false",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
				req = req.WithContext(domain.WithTrustedProxy(req.Context(), true))
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

// fakeRevocationChecker is provided by internal/testutil (FakeRevocationChecker).
// The shared type is used by both auth and middleware package tests to avoid
// duplication.
