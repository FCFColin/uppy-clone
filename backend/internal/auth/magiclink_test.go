package auth

// 企业为何需要：安全关键组件（中间件/认证/管理）零测试是最高风险——任何改动都可在生产暴露。

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const testPlayerNickname = "TestPlayer"

// --- MagicLinkService tests ---
// Note: RequestMagicLink and VerifyMagicLink use concrete *store.RedisStore,
// so we test the pure logic functions they depend on, and test the full flow
// via integration tests with real Redis.

func TestRequestMagicLink_InvalidEmail(t *testing.T) {
	tests := []struct {
		email string
		valid bool
	}{
		{"invalid-email", false},
		{"@domain.com", false},
		{"user@", false},
		{"", false},
		{"user@domain", false},
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

func TestGetOrigin(t *testing.T) {
	tests := []struct {
		name  string
		setup func(r *http.Request)
		want  string
	}{
		{
			name:  "plain HTTP",
			setup: func(r *http.Request) { r.Host = "localhost:8080" },
			want:  "http://localhost:8080",
		},
		{
			name: "TLS connection",
			setup: func(r *http.Request) {
				r.Host = "example.com"
				r.TLS = &tls.ConnectionState{}
			},
			want: "https://example.com",
		},
		{
			name: "X-Forwarded-Proto and X-Forwarded-Host",
			setup: func(r *http.Request) {
				r.Host = "internal-host"
				r.Header.Set("X-Forwarded-Proto", "https")
				r.Header.Set("X-Forwarded-Host", "public.example.com")
			},
			want: "https://public.example.com",
		},
		{
			name: "X-Forwarded-Proto only",
			setup: func(r *http.Request) {
				r.Host = "myhost"
				r.Header.Set("X-Forwarded-Proto", "https")
			},
			want: "https://myhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			tt.setup(r)
			got := getOrigin(r)
			if got != tt.want {
				t.Errorf("getOrigin() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMagicTokenData_Serialization(t *testing.T) {
	data := magicTokenData{
		Email:     "user@example.com",
		CreatedAt: time.Now().UnixMilli(),
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed magicTokenData
	if err := json.Unmarshal(bytes, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if parsed.Email != data.Email {
		t.Errorf("email = %q, want %q", parsed.Email, data.Email)
	}
	if parsed.CreatedAt != data.CreatedAt {
		t.Errorf("createdAt = %d, want %d", parsed.CreatedAt, data.CreatedAt)
	}
}

func TestVerifyResponse_Structure(t *testing.T) {
	resp := VerifyResponse{
		UserID:       "user-123",
		Nickname:     testPlayerNickname,
		RefreshToken: "refresh-token-abc",
	}

	if resp.UserID != "user-123" {
		t.Errorf("UserID = %q, want %q", resp.UserID, "user-123")
	}
	if resp.Nickname != testPlayerNickname {
		t.Errorf("Nickname = %q, want %q", resp.Nickname, testPlayerNickname)
	}
	if resp.RefreshToken != "refresh-token-abc" {
		t.Errorf("RefreshToken = %q, want %q", resp.RefreshToken, "refresh-token-abc")
	}
}
