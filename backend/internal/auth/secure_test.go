package auth

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 企业为何需要：Cookie 的 Secure 标志在反向代理后失效会导致中间人攻击。
// 这些测试覆盖直连 HTTPS、X-Forwarded-Proto 头、以及无 TLS 场景，确保反向代理下 Secure 标志正确设置。
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
			name: "X-Forwarded-Proto: https returns true",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
				req.Header.Set("X-Forwarded-Proto", "https")
				return req
			},
			want: true,
		},
		{
			name: "X-Forwarded-Proto: http returns false",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
				req.Header.Set("X-Forwarded-Proto", "http")
				return req
			},
			want: false,
		},
		{
			name: "no TLS, no header returns false",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
				return req
			},
			want: false,
		},
		{
			name: "empty X-Forwarded-Proto returns false",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
				req.Header.Set("X-Forwarded-Proto", "")
				return req
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupReq()
			got := IsSecure(req)
			if got != tt.want {
				t.Errorf("IsSecure() = %v, want %v", got, tt.want)
			}
		})
	}
}
