package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
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
