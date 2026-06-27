package auth

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uppy-clone/backend/internal/requestctx"
)

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
