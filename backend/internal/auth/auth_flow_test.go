// Package auth provides authentication and authorization utilities.
package auth

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/nicknames"
	"github.com/uppy-clone/backend/internal/testsecrets"
)

// 企业为何需要：安全关键组件（中间件/认证/管理）零测试是最高风险——任何改动都可在生产暴露。

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
				*r = *r.WithContext(domain.WithTrustedProxy(r.Context(), true))
				r.Host = "internal-host"
				r.Header.Set("X-Forwarded-Proto", "https")
				r.Header.Set("X-Forwarded-Host", "public.example.com")
			},
			want: "https://public.example.com",
		},
		{
			name: "X-Forwarded-Proto only",
			setup: func(r *http.Request) {
				*r = *r.WithContext(domain.WithTrustedProxy(r.Context(), true))
				r.Host = "myhost"
				r.Header.Set("X-Forwarded-Proto", "https")
			},
			want: "https://myhost",
		},
		{
			name: "spoofed X-Forwarded-Proto ignored when untrusted",
			setup: func(r *http.Request) {
				r.Host = "myhost"
				r.Header.Set("X-Forwarded-Proto", "https")
			},
			want: "http://myhost",
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

// 企业为何需要：安全关键组件（中间件/认证/管理）零测试是最高风险——任何改动都可在生产暴露。

// --- QuickPlay pure logic tests ---

func TestSanitizePlayerName(t *testing.T) {
	t.Run("removes control chars", func(t *testing.T) {
		result := sanitizePlayerName("hello\x00world\x01test")
		if strings.Contains(result, "\x00") || strings.Contains(result, "\x01") {
			t.Errorf("sanitizePlayerName should remove control chars, got %q", result)
		}
	})

	t.Run("removes zero-width chars", func(t *testing.T) {
		result := sanitizePlayerName("hello\u200Bworld")
		if strings.Contains(result, "\u200B") {
			t.Errorf("sanitizePlayerName should remove zero-width chars, got %q", result)
		}
	})

	t.Run("removes HTML chars", func(t *testing.T) {
		result := sanitizePlayerName("hello<script>alert('xss')</script>")
		if strings.Contains(result, "<") || strings.Contains(result, ">") {
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

	t.Run("removes dangerous HTML chars", func(t *testing.T) {
		result := sanitizePlayerName("hello&world")
		if strings.Contains(result, "&") {
			t.Errorf("sanitizePlayerName should remove & char, got %q", result)
		}
	})

	t.Run("empty string returns empty", func(t *testing.T) {
		result := sanitizePlayerName("")
		if result != "" {
			t.Errorf("sanitizePlayerName of empty should be empty, got %q", result)
		}
	})
}

func TestQuickPlayRandomNickname(t *testing.T) {
	t.Run("produces non-empty nickname from word pool", func(t *testing.T) {
		name := nicknames.GenerateRandom(nil)
		if name == "" {
			t.Fatalf("nicknames.GenerateRandom = empty")
		}
	})
}

func TestGenerateUUID(t *testing.T) {
	t.Run("produces valid format", func(t *testing.T) {
		id := domain.UUID()
		if len(id) != 36 {
			t.Errorf("domain.UUID length = %d, want 36", len(id))
		}
		parts := strings.Split(id, "-")
		if len(parts) != 5 {
			t.Errorf("domain.UUID should have 5 dash-separated parts, got %d", len(parts))
		}
	})

	t.Run("produces unique IDs", func(t *testing.T) {
		id1 := domain.UUID()
		id2 := domain.UUID()
		if id1 == id2 {
			t.Error("domain.UUID should produce unique IDs")
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

	t.Run("returns empty string for empty body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
		nickname := ParseQuickPlayRequest(req)
		if nickname != "" {
			t.Errorf("ParseQuickPlayRequest = %q, want empty string for empty body", nickname)
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

func TestQuickPlay_DuplicateUser(t *testing.T) {
	t.Run("GetAuthenticatedUser returns false for unauthenticated request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		_, _, ok := GetAuthenticatedUser(req)
		if ok {
			t.Error("GetAuthenticatedUser should return false for unauthenticated request")
		}
	})

	t.Run("AuthenticatedUserFromRequest reads quickplay cookie", func(t *testing.T) {
		jwtMgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
		token, err := jwtMgr.SignToken("user-cookie", "Nick")
		if err != nil {
			t.Fatalf("SignToken() error = %v", err)
		}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: "quickplay", Value: token})
		uid, nick, ok := AuthenticatedUserFromRequestWithRevocation(req, jwtMgr, nil)
		if !ok || uid != "user-cookie" || nick != "Nick" {
			t.Fatalf("AuthenticatedUserFromRequestWithRevocation = (%q, %q, %v), want (user-cookie, Nick, true)", uid, nick, ok)
		}
	})
}

func TestBuildAuthCookie(t *testing.T) {
	t.Run("creates HttpOnly cookie", func(t *testing.T) {
		cookie := BuildAuthCookie("test", "value", 900, true)
		if !cookie.HttpOnly {
			t.Error("cookie should be HttpOnly")
		}
	})

	t.Run("creates Secure cookie when secure=true", func(t *testing.T) {
		cookie := BuildAuthCookie("test", "value", 900, true)
		if !cookie.Secure {
			t.Error("cookie should be Secure")
		}
	})

	t.Run("creates non-Secure cookie when secure=false", func(t *testing.T) {
		cookie := BuildAuthCookie("test", "value", 900, false)
		if cookie.Secure {
			t.Error("cookie should not be Secure")
		}
	})

	t.Run("sets SameSite Lax", func(t *testing.T) {
		cookie := BuildAuthCookie("test", "value", 900, true)
		if cookie.SameSite != http.SameSiteLaxMode {
			t.Errorf("SameSite = %v, want %v", cookie.SameSite, http.SameSiteLaxMode)
		}
	})

	t.Run("sets MaxAge", func(t *testing.T) {
		cookie := BuildAuthCookie("test", "value", 900, true)
		if cookie.MaxAge != 900 {
			t.Errorf("MaxAge = %d, want 900", cookie.MaxAge)
		}
	})

	t.Run("sets Path to root", func(t *testing.T) {
		cookie := BuildAuthCookie("test", "value", 900, true)
		if cookie.Path != "/" {
			t.Errorf("Path = %q, want %q", cookie.Path, "/")
		}
	})
}

func TestQuickPlayResponse_Structure(t *testing.T) {
	resp := QuickPlayResponse{
		UserID:       "user-123",
		Nickname:     "TestPlayer",
		RefreshToken: "refresh-abc",
	}
	if resp.UserID != "user-123" {
		t.Errorf("UserID = %q, want %q", resp.UserID, "user-123")
	}
}
