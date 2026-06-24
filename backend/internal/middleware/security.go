package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5/middleware"
)

// nonceCtxKey is the context key for the CSP nonce.
type nonceCtxKey struct{}

// GetRequestID extracts the chi request_id from context.
// Returns empty string if not found.
func GetRequestID(ctx context.Context) string {
	return middleware.GetReqID(ctx)
}

// SecurityHeaders adds security-related HTTP response headers to all responses.
//
// Enterprise rationale: Security headers are a defense-in-depth measure.
// - HSTS (RFC 6797): Forces HTTPS for subsequent visits, prevents SSL stripping
// - X-Content-Type-Options: Prevents MIME type sniffing
// - X-Frame-Options: Prevents clickjacking
// - CSP: Controls resource loading (XSS mitigation)
// - Referrer-Policy: Controls referrer information leakage
// - Permissions-Policy: Disables unnecessary browser APIs
// Trade-off: HSTS requires HTTPS deployment; CSP may break inline scripts.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 企业为何需要：HSTS 防止 SSL 剥离攻击。默认启用确保生产环境安全，开发环境可通过 ENABLE_HSTS=false 关闭。
		if os.Getenv("ENABLE_HSTS") != "false" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// 企业为何需要：Permissions-Policy 禁用浏览器不需要的 API，减少攻击面。
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		// 企业为何需要：CSP unsafe-inline 削弱 XSS 防御。Nonce-based CSP 允许已知脚本执行，阻止注入的恶意脚本。
		// 权衡：style-src 保留 unsafe-inline 因为 Vite 注入的样式需要它。
		nonce := generateNonce()
		csp := "script-src 'self' 'nonce-" + nonce + "'; " +
			"style-src 'self' 'unsafe-inline'; " +
			"connect-src 'self' wss: ws:; " +
			"img-src 'self' data:; " +
			"default-src 'self'"
		w.Header().Set("Content-Security-Policy", csp)

		// Store nonce in context so templates can use it
		ctx := context.WithValue(r.Context(), nonceCtxKey{}, nonce)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// generateNonce creates a cryptographically secure random nonce (16 bytes, hex-encoded).
func generateNonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback: should never happen with crypto/rand
		return "fallback-nonce"
	}
	return hex.EncodeToString(b)
}
