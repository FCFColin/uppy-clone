package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ─── SignToken + VerifyToken round-trip ──────────────────────────────

func TestSignVerifyToken_RoundTrip(t *testing.T) {
	mgr := NewJWTManager("test-secret-key-padded-to-32-bytes!!")

	token, err := mgr.SignToken("user-123", "快乐的气球")
	if err != nil {
		t.Fatalf("SignToken 失败: %v", err)
	}

	userId, nickname, jti, err := mgr.VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken 失败: %v", err)
	}
	if userId != "user-123" {
		t.Fatalf("userId 不匹配: got=%s, want=user-123", userId)
	}
	if nickname != "快乐的气球" {
		t.Fatalf("nickname 不匹配: got=%s, want=快乐的气球", nickname)
	}
	if jti == "" {
		t.Fatal("jti 不应为空")
	}
	if len(jti) != 32 {
		t.Fatalf("jti 应为 32 字符 hex (16 bytes); got len=%d", len(jti))
	}
}

// ─── JTI uniqueness ──────────────────────────────────────────────────

func TestSignToken_JTIUnique(t *testing.T) {
	mgr := NewJWTManager("test-secret-key-padded-to-32-bytes!!")

	token1, _ := mgr.SignToken("user-1", "alice")
	token2, _ := mgr.SignToken("user-1", "alice")

	_, _, jti1, _ := mgr.VerifyToken(token1)
	_, _, jti2, _ := mgr.VerifyToken(token2)

	if jti1 == jti2 {
		t.Fatal("两个 token 的 jti 应不同")
	}
}

// ─── Expired token ───────────────────────────────────────────────────

func TestVerifyToken_Expired(t *testing.T) {
	mgr := NewJWTManager("test-secret-key-padded-to-32-bytes!!")

	// 手动构造一个已过期的 token
	claims := customClaims{
		Nickname: "test",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-exp",
			ID:        "test-jti-expired",
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-8 * 24 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Second)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(mgr.primarySecret)
	if err != nil {
		t.Fatalf("签名失败: %v", err)
	}

	_, _, _, err = mgr.VerifyToken(tokenString)
	if err == nil {
		t.Fatal("已过期的 token 应验证失败")
	}
}

// ─── Invalid token ───────────────────────────────────────────────────

func TestVerifyToken_Invalid(t *testing.T) {
	mgr := NewJWTManager("test-secret-key-padded-to-32-bytes!!")

	_, _, _, err := mgr.VerifyToken("this.is.not.a.valid.token")
	if err == nil {
		t.Fatal("无效 token 应验证失败")
	}
}

func TestVerifyToken_WrongSecret(t *testing.T) {
	mgr1 := NewJWTManager("secret-1-padded-to-32-bytes!!!!!!!!")
	mgr2 := NewJWTManager("secret-2-padded-to-32-bytes!!!!!!!!")

	token, err := mgr1.SignToken("user-1", "test")
	if err != nil {
		t.Fatalf("SignToken 失败: %v", err)
	}

	_, _, _, err = mgr2.VerifyToken(token)
	if err == nil {
		t.Fatal("使用错误密钥验证应失败")
	}
}

// ─── BuildAuthCookie ─────────────────────────────────────────────────

func TestBuildAuthCookie_HttpOnly(t *testing.T) {
	cookie := BuildAuthCookie("auth_token", "jwt-value", 3600, true)

	if !cookie.HttpOnly {
		t.Fatal("cookie 应设置 HttpOnly=true")
	}
	if cookie.Name != "auth_token" {
		t.Fatalf("cookie name 不匹配: got=%s, want=auth_token", cookie.Name)
	}
	if cookie.Value != "jwt-value" {
		t.Fatalf("cookie value 不匹配: got=%s, want=jwt-value", cookie.Value)
	}
	if cookie.Path != "/" {
		t.Fatalf("cookie path 不匹配: got=%s, want=/", cookie.Path)
	}
	if cookie.MaxAge != 3600 {
		t.Fatalf("cookie MaxAge 不匹配: got=%d, want=3600", cookie.MaxAge)
	}
	if !cookie.Secure {
		t.Fatal("cookie 应设置 Secure=true")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie SameSite 应为 Lax，got=%v", cookie.SameSite)
	}
}

// ─── ParseAuthCookie ─────────────────────────────────────────────────

func TestParseAuthCookie_Valid(t *testing.T) {
	mgr := NewJWTManager("test-secret-padded-to-32-bytes!!!!")

	token, _ := mgr.SignToken("user-1", "nickname")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  "auth",
		Value: token,
	})

	userId, nickname, jti, err := ParseAuthCookie(req, "auth", mgr)
	if err != nil {
		t.Fatalf("ParseAuthCookie 失败: %v", err)
	}
	if userId != "user-1" {
		t.Fatalf("userId 不匹配: got=%s, want=user-1", userId)
	}
	if nickname != "nickname" {
		t.Fatalf("nickname 不匹配: got=%s, want=nickname", nickname)
	}
	if jti == "" {
		t.Fatal("jti 不应为空")
	}
}

func TestParseAuthCookie_Missing(t *testing.T) {
	mgr := NewJWTManager("test-secret-padded-to-32-bytes!!!!")

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	_, _, _, err := ParseAuthCookie(req, "auth", mgr)
	if err == nil {
		t.Fatal("缺少 cookie 应返回错误")
	}
}

// ─── generateJTI ─────────────────────────────────────────────────────

func TestGenerateJTI(t *testing.T) {
	jti1, err := generateJTI()
	if err != nil {
		t.Fatalf("generateJTI 失败: %v", err)
	}
	if len(jti1) != 32 {
		t.Fatalf("jti 长度应为 32; got %d", len(jti1))
	}

	jti2, _ := generateJTI()
	if jti1 == jti2 {
		t.Fatal("两次生成的 jti 应不同")
	}
}
