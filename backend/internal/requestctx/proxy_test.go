package requestctx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithTrustedProxy(t *testing.T) {
	t.Parallel()
	ctx := WithTrustedProxy(context.Background(), true)
	if !IsTrustedProxy(ctx) {
		t.Error("IsTrustedProxy should return true after WithTrustedProxy(ctx, true)")
	}
}

func TestWithTrustedProxy_False(t *testing.T) {
	t.Parallel()
	ctx := WithTrustedProxy(context.Background(), false)
	if IsTrustedProxy(ctx) {
		t.Error("IsTrustedProxy should return false after WithTrustedProxy(ctx, false)")
	}
}

func TestIsTrustedProxy_EmptyContext(t *testing.T) {
	t.Parallel()
	if IsTrustedProxy(context.Background()) {
		t.Error("IsTrustedProxy should return false on empty context")
	}
}

func TestIsTrustedProxy_WrongValueType(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), proxyKey{}, "yes")
	if IsTrustedProxy(ctx) {
		t.Error("IsTrustedProxy should return false for non-bool value")
	}
}

func TestIsTrustedProxy_Override(t *testing.T) {
	t.Parallel()
	ctx := WithTrustedProxy(context.Background(), true)
	ctx = WithTrustedProxy(ctx, false)
	if IsTrustedProxy(ctx) {
		t.Error("IsTrustedProxy should return false after override with false")
	}
}

func TestExtractClientIP_RemoteAddr(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.10:54321"
	if got := ExtractClientIP(req); got != "192.168.1.10" {
		t.Fatalf("ExtractClientIP = %q", got)
	}
}

func TestExtractClientIP_RemoteAddrNoPort(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.10"
	if got := ExtractClientIP(req); got != "192.168.1.10" {
		t.Fatalf("ExtractClientIP = %q", got)
	}
}

func TestExtractClientIP_TrustedXFF(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(WithTrustedProxy(req.Context(), true))
	req.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1")
	req.RemoteAddr = "127.0.0.1:8080"
	if got := ExtractClientIP(req); got != "203.0.113.5" {
		t.Fatalf("ExtractClientIP = %q", got)
	}
}

func TestExtractClientIP_TrustedEmptyXFF(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(WithTrustedProxy(req.Context(), true))
	req.RemoteAddr = "10.0.0.2:1234"
	if got := ExtractClientIP(req); got != "10.0.0.2" {
		t.Fatalf("ExtractClientIP = %q", got)
	}
}
