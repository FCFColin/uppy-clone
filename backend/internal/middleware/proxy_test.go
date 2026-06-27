package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrustedProxy_StripsUntrustedForwardedHeaders(t *testing.T) {
	var seenProto, seenXFF string
	handler := TrustedProxy("127.0.0.1/32")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenProto = r.Header.Get("X-Forwarded-Proto")
		seenXFF = r.Header.Get("X-Forwarded-For")
		if !IsTrustedProxy(r) {
			t.Error("expected trusted proxy when peer is 127.0.0.1")
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if seenProto != "https" {
		t.Fatalf("X-Forwarded-Proto = %q, want https", seenProto)
	}
	if seenXFF != "203.0.113.1" {
		t.Fatalf("X-Forwarded-For = %q, want 203.0.113.1", seenXFF)
	}
}

func TestTrustedProxy_StripsSpoofedHeadersFromUntrustedPeer(t *testing.T) {
	var seenProto string
	handler := TrustedProxy("10.0.0.0/8")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenProto = r.Header.Get("X-Forwarded-Proto")
		if IsTrustedProxy(r) {
			t.Error("expected untrusted proxy for public peer")
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.50:1234"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if seenProto != "" {
		t.Fatalf("X-Forwarded-Proto = %q, want empty", seenProto)
	}
}

func TestExtractClientIP_UsesRemoteAddrWhenUntrusted(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.50:4321"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	if got := ExtractClientIP(req); got != "203.0.113.50" {
		t.Fatalf("ExtractClientIP() = %q, want 203.0.113.50", got)
	}
}

func TestIsOriginAllowed_ExactMatch(t *testing.T) {
	allowed := []string{"https://app.example.com", "http://localhost:5173"}
	if !IsOriginAllowed("https://app.example.com", allowed) {
		t.Fatal("expected exact origin match")
	}
	// Adversarial: hostname-only match must fail
	if IsOriginAllowed("https://evil.example.com", allowed) {
		t.Fatal("hostname-only match must not pass")
	}
}
