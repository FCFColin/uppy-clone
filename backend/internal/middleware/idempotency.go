package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/config"
)

type idempCtxKey struct{}

// 企业为何需要：幂等性是分布式系统的关键保证。网络重试导致重复创建资源（如重复扣款）
// 是经典生产事故。Idempotency-Key 是 RFC 草案标准。

// defaultIdempotencyTTL is the default TTL for cached idempotent responses.
const defaultIdempotencyTTL = 24 * time.Hour

// idempotencyCachedResponse is the structure stored in Redis for cached responses.
type idempotencyCachedResponse struct {
	StatusCode int    `json:"status_code"`
	Body       string `json:"body"`
}

// responseRecorder wraps http.ResponseWriter to capture the response body and status code.
type responseRecorder struct {
	http.ResponseWriter
	body       bytes.Buffer
	statusCode int
	written    bool
}

func newResponseRecorder(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK, // default if WriteHeader is never called
	}
}

func (r *responseRecorder) WriteHeader(code int) {
	if r.written {
		return
	}
	r.statusCode = code
	r.written = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.written {
		r.statusCode = http.StatusOK
		r.written = true
	}
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

// IdempotencyMiddleware checks the Idempotency-Key header and returns
// cached response if the same key was used before. On successful handler
// execution (2xx), the response is automatically cached in Redis so that
// retried requests receive the same response without re-executing the handler.
//
// Enterprise rationale: Prevents duplicate resource creation when clients
// retry due to network failures. Common in payment systems and any
// non-idempotent POST endpoint. Trade-off: Extra Redis round-trip per
// request, but prevents duplicate rooms/charges.
func IdempotencyMiddleware(rdb *redis.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("Idempotency-Key")
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			if len(key) > config.IdempotencyKeyMaxLen {
				apierror.BadRequest("idempotency key too long").Write(w)
				return
			}

			// Hash the key to prevent injection
			hash := sha256.Sum256([]byte(key))
			redisKey := "idem:" + hex.EncodeToString(hash[:])

			// Check if we've seen this key before
			val, err := rdb.Get(r.Context(), redisKey).Result()
			if err == nil {
				// Key exists — return cached response
				var cached idempotencyCachedResponse
				if decodeErr := json.Unmarshal([]byte(val), &cached); decodeErr != nil {
					slog.Error("idempotency: failed to decode cached response", "key_hash", redisKey, "error", decodeErr)
					// Fall through to execute handler normally
				} else {
					slog.Info("idempotency: returning cached response", "key_hash", redisKey)
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("X-Idempotent-Replayed", "true")
					w.WriteHeader(cached.StatusCode)
				_, _ = w.Write([]byte(cached.Body))
					return
				}
			}

			// No cached response — execute handler and capture response
			recorder := newResponseRecorder(w)
			// Store key in context so handler can access it if needed
			ctx := context.WithValue(r.Context(), idempCtxKey{}, redisKey)
			next.ServeHTTP(recorder, r.WithContext(ctx))

			// Cache successful responses (2xx) automatically
			if recorder.statusCode >= 200 && recorder.statusCode < 300 {
				if saveErr := SaveIdempotencyResponse(r.Context(), rdb, redisKey, recorder.statusCode, recorder.body.Bytes(), defaultIdempotencyTTL); saveErr != nil {
					slog.Error("idempotency: failed to save response", "key_hash", redisKey, "error", saveErr)
				}
			}
		})
	}
}

// SaveIdempotencyResponse stores the response in Redis for future idempotent replays.
// This is called automatically by the middleware for 2xx responses, but can also be
// called manually by handlers that need custom caching behavior.
func SaveIdempotencyResponse(ctx context.Context, rdb *redis.Client, key string, statusCode int, body []byte, ttl time.Duration) error {
	cached := idempotencyCachedResponse{
		StatusCode: statusCode,
		Body:       string(body),
	}
	data, err := json.Marshal(cached)
	if err != nil {
		return err
	}
	return rdb.Set(ctx, key, data, ttl).Err()
}

// GetIdempotencyKey returns the Redis key for storing the idempotent response.
func GetIdempotencyKey(ctx context.Context) string {
	if key, ok := ctx.Value(idempCtxKey{}).(string); ok {
		return key
	}
	return ""
}
