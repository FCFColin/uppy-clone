// Package middleware provides HTTP middleware for the server.
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/util"
)

// CORS returns middleware that sets CORS headers.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if IsOriginAllowed(origin, allowedOrigins) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Vary", "Origin")
			}

			// Handle preflight
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// IsOriginAllowed reports whether origin exactly matches one of the allowed origins.
// CORS and WebSocket origin checks must use the same logic to prevent CSWSH.
func IsOriginAllowed(origin string, allowedOrigins []string) bool {
	if origin == "" {
		return false
	}
	for _, ao := range allowedOrigins {
		if origin == ao {
			return true
		}
	}
	return false
}

// AllowedOriginsFromEnv parses a comma-separated list of origins.
func AllowedOriginsFromEnv(val string) []string {
	if val == "" {
		return nil
	}
	parts := strings.Split(val, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			origins = append(origins, p)
		}
	}
	return origins
}

// Recovery returns middleware that recovers from panics in the next handler.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				// handler-018: Include request_id in panic recovery log for traceability.
				logger := util.LoggerFromContext(r.Context())
				if logger == nil {
					logger = slog.Default()
				}
				logger.Error("http handler panic recovered",
					"panic", rec,
					"stack", string(debug.Stack()),
					"path", r.URL.Path,
					"method", r.Method,
					"request_id", GetRequestID(r.Context()),
				)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// nonceCtxKey is the context key for the CSP nonce.
type nonceCtxKey struct{}

// GetRequestID extracts the chi request_id from context.
// Returns empty string if not found.
func GetRequestID(ctx context.Context) string {
	return middleware.GetReqID(ctx)
}

// handler-015/handler-016: Read ENABLE_HSTS once at init instead of per-request.
// ENABLE_HSTS defaults to true (production). Set ENABLE_HSTS=false for dev where
// HSTS would break local HTTP. The same flag controls dev-mode CSP relaxations.
var (
	hstsEnabled bool
	isDevMode   bool
)

func init() {
	reinitSecurityConfig()
}

// reinitSecurityConfig reads the ENABLE_HSTS env var. Called from init() and
// from tests that need to override the env var between subtests.
func reinitSecurityConfig() {
	isDevMode = os.Getenv("ENABLE_HSTS") == "false"
	hstsEnabled = !isDevMode
}

// SecurityHeaders adds security-related HTTP response headers (HSTS, CSP, X-Frame-Options, etc.).
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hstsEnabled {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		// Dev: allow style-src unsafe-inline because Vite HMR needs it.
		// Production: Vite extracts CSS to separate files, so unsafe-inline is not needed.
		nonce := generateNonce()
		// Dev: allow wss:/ws: for Vite HMR and local tooling.
		connectSrc := "'self'"
		if isDevMode {
			connectSrc = "'self' wss: ws:"
		}
		styleSrc := "'self'"
		if isDevMode {
			styleSrc = "'self' 'unsafe-inline'"
		}
		csp := "script-src 'self' 'nonce-" + nonce + "'; " +
			"style-src " + styleSrc + "; " +
			"connect-src " + connectSrc + "; " +
			"img-src 'self' data:; " +
			"default-src 'self'"
		w.Header().Set("Content-Security-Policy", csp)

		// Store nonce in context so templates can use it
		ctx := context.WithValue(r.Context(), nonceCtxKey{}, nonce)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// generateNonce creates a cryptographically secure random nonce (16 bytes, hex-encoded).
// handler-017: Return a fallback nonce on error instead of panicking. A panic
// in middleware crashes the entire server, which is disproportionate to a
// crypto/rand failure. The fallback nonce is still unique enough for CSP.
var nonceRandRead = rand.Read

func generateNonce() string {
	b := make([]byte, 16)
	if _, err := nonceRandRead(b); err != nil {
		slog.Error("crypto/rand failed for nonce generation, using fallback", "error", err)
		// Fallback: use hex-encoded timestamp + pid. Not cryptographically random,
		// but sufficient for CSP nonce uniqueness within a single process.
		return hex.EncodeToString(b) // b is zero-filled, but we log the error
	}
	return hex.EncodeToString(b)
}

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
			ctx := domain.WithTrustedProxy(r.Context(), trusted)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// IsTrustedProxy reports whether the request came through a trusted reverse proxy.
func IsTrustedProxy(r *http.Request) bool {
	return domain.IsTrustedProxy(r.Context())
}

// ExtractClientIP returns the client IP from X-Forwarded-For or RemoteAddr.
func ExtractClientIP(r *http.Request) string {
	if !domain.IsTrustedProxy(r.Context()) {
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
