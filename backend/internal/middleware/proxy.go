package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/uppy-clone/backend/internal/requestctx"
)

// TrustedProxy strips or honors X-Forwarded-* headers based on whether the
// immediate peer (RemoteAddr) is in the trusted CIDR list.
func TrustedProxy(trustedCIDRs string) func(http.Handler) http.Handler {
	nets := parseTrustedCIDRs(trustedCIDRs)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			trusted := isTrustedPeer(r, nets)
			if !trusted {
				r.Header.Del("X-Forwarded-For")
				r.Header.Del("X-Forwarded-Proto")
				r.Header.Del("X-Forwarded-Host")
			}
			ctx := requestctx.WithTrustedProxy(r.Context(), trusted)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// IsTrustedProxy reports whether the request came through a trusted reverse proxy.
func IsTrustedProxy(r *http.Request) bool {
	return requestctx.IsTrustedProxy(r.Context())
}

func parseTrustedCIDRs(raw string) []*net.IPNet {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var nets []*net.IPNet
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !strings.Contains(part, "/") {
			// handler-019: Use /128 for IPv6 single IPs, /32 for IPv4.
			if strings.Contains(part, ":") {
				part += "/128"
			} else {
				part += "/32"
			}
		}
		_, n, err := net.ParseCIDR(part)
		if err == nil {
			nets = append(nets, n)
		}
	}
	return nets
}

func isTrustedPeer(r *http.Request, nets []*net.IPNet) bool {
	if len(nets) == 0 {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
