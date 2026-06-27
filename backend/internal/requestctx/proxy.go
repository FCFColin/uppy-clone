package requestctx

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type proxyKey struct{}

// WithTrustedProxy marks whether the request peer is a trusted reverse proxy.
func WithTrustedProxy(ctx context.Context, trusted bool) context.Context {
	return context.WithValue(ctx, proxyKey{}, trusted)
}

// IsTrustedProxy reports whether X-Forwarded-* headers were honored for this request.
func IsTrustedProxy(ctx context.Context) bool {
	v, ok := ctx.Value(proxyKey{}).(bool)
	return ok && v
}

// ExtractClientIP returns the client IP from X-Forwarded-For or RemoteAddr.
func ExtractClientIP(r *http.Request) string {
	if !IsTrustedProxy(r.Context()) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			return r.RemoteAddr
		}
		return ip
	}

	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		ips := strings.SplitN(xff, ",", 2)
		ip := strings.TrimSpace(ips[0])
		if ip != "" {
			return ip
		}
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
