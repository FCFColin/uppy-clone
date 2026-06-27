package auth

// 企业为何需要：安全关键组件（中间件/认证/管理）零测试是最高风险——任何改动都可在生产暴露。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/uppy-clone/backend/internal/idgen"
)

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

func TestGenerateRandomPlayerName(t *testing.T) {
	t.Run("produces Player prefix", func(t *testing.T) {
		name := generateRandomPlayerName()
		if !strings.HasPrefix(name, "Player") {
			t.Errorf("generateRandomPlayerName = %q, want Player prefix", name)
		}
	})
}

func TestGenerateUUID(t *testing.T) {
	t.Run("produces valid format", func(t *testing.T) {
		id := idgen.UUID()
		if len(id) != 36 {
			t.Errorf("idgen.UUID length = %d, want 36", len(id))
		}
		parts := strings.Split(id, "-")
		if len(parts) != 5 {
			t.Errorf("idgen.UUID should have 5 dash-separated parts, got %d", len(parts))
		}
	})

	t.Run("produces unique IDs", func(t *testing.T) {
		id1 := idgen.UUID()
		id2 := idgen.UUID()
		if id1 == id2 {
			t.Error("idgen.UUID should produce unique IDs")
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
		jwtMgr := NewJWTManager("test-secret-key-0123456789abcdef0123456789")
		token, err := jwtMgr.SignToken("user-cookie", "Nick")
		if err != nil {
			t.Fatalf("SignToken() error = %v", err)
		}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: "quickplay", Value: token})
		uid, nick, ok := AuthenticatedUserFromRequest(req, jwtMgr)
		if !ok || uid != "user-cookie" || nick != "Nick" {
			t.Fatalf("AuthenticatedUserFromRequest = (%q, %q, %v), want (user-cookie, Nick, true)", uid, nick, ok)
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
