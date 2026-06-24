package auth

import "net/http"

// IsSecure reports whether the request was made over HTTPS.
// It checks X-Forwarded-Proto first (for reverse proxies like nginx/cloudflare),
// then falls back to r.TLS.
//
// 企业为何需要：r.URL.Scheme 在 net/http 中几乎总是空字符串，导致反向代理后 Cookie 的 Secure 标志失效，
// 浏览器会在 HTTP 下发送 Cookie 造成中间人攻击风险。导出为 IsSecure 以便 handler 包调用。
func IsSecure(r *http.Request) bool {
	if proto := r.Header.Get("X-Forwarded-Proto"); proto == "https" {
		return true
	}
	return r.TLS != nil
}
