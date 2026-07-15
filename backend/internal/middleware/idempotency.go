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

// IdempotencyStore is the narrow subset of *redis.Client methods used by
// IdempotencyMiddleware. Abstracting behind an interface (RO-051) prevents
// raw *redis.Client penetration — the middleware receives a contract, not the
// full client. *redis.Client satisfies this interface automatically.
type IdempotencyStore interface {
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.BoolCmd
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

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
// TODO: The SETNX-based idempotency middleware path (claim, exec, cache) has no dedicated unit tests.
// The middleware integrates with Redis via SETNX for atomic key claiming, then caches 2xx responses
// for idempotent replay. Non-2xx responses delete the key to allow retry. Testing this path requires
// a Redis instance or mock, which the current test suite does not provide. Key areas for test coverage:
//  1. SETNX claim succeeds → handler executes → response cached on 2xx
//  2. SETNX claim succeeds → handler executes → key deleted on non-2xx
//  3. SETNX claim fails (key exists, status "processing") → 409 Conflict returned
//  4. SETNX claim fails → cached response exists → replay cached response with X-Idempotent-Replayed header
//  5. SETNX claim fails → cached response malformed JSON → 409 Conflict returned
//  6. Empty/missing Idempotency-Key header → pass-through without Redis interaction
//  7. Idempotency-Key exceeds max length → 400 Bad Request
func IdempotencyMiddleware(rdb IdempotencyStore) func(http.Handler) http.Handler {
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

			redisKey := buildIdempotencyKey(key)

			// Use SETNX to atomically claim the idempotency key
			claimed, err := rdb.SetNX(r.Context(), redisKey, "processing", defaultIdempotencyTTL).Result()
			if err != nil {
				slog.Error("idempotency: failed to claim key, denying request", "key_hash", redisKey, "error", err)
				apierror.New(http.StatusServiceUnavailable, "Service Unavailable",
					"Request cannot be processed at this time, please retry later").Write(w)
				return
			}

			// Inject the idempotency key into context for downstream handlers
			ctx := context.WithValue(r.Context(), idempCtxKey{}, redisKey)
			r = r.WithContext(ctx)

			if !claimed {
				// Key exists — replay cached response if available, else 409.
				if replayCachedResponse(w, rdb, r.Context(), redisKey) {
					return
				}
				apierror.New(http.StatusConflict, "Request with this idempotency key is already in progress", "conflict").Write(w)
				return
			}

			// Claimed the key — execute handler and capture response
			recorder := newResponseRecorder(w)
			next.ServeHTTP(recorder, r)
			cacheOrDeleteIdempotencyResponse(r, rdb, redisKey, recorder)
		})
	}
}

// buildIdempotencyKey hashes the raw idempotency key to a Redis-safe identifier
// to prevent injection and bound key size.
func buildIdempotencyKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return "idem:" + hex.EncodeToString(hash[:])
}

// replayCachedResponse attempts to replay a previously cached response for the
// given idempotency key. Returns true if a cached response was successfully
// written to w; false if no cached response exists, the value is still
// "processing", or the cached payload is malformed (caller should return 409).
func replayCachedResponse(w http.ResponseWriter, rdb IdempotencyStore, ctx context.Context, redisKey string) bool {
	val, err := rdb.Get(ctx, redisKey).Result()
	if err != nil || val == "processing" {
		return false
	}
	var cached idempotencyCachedResponse
	if decodeErr := json.Unmarshal([]byte(val), &cached); decodeErr != nil {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Idempotent-Replayed", "true")
	w.WriteHeader(cached.StatusCode)
	_, _ = w.Write([]byte(cached.Body))
	return true
}

// cacheOrDeleteIdempotencyResponse caches 2xx responses for idempotent replay,
// or deletes the idempotency key on non-2xx so the client can retry with the
// same key.
func cacheOrDeleteIdempotencyResponse(r *http.Request, rdb IdempotencyStore, redisKey string, recorder *responseRecorder) {
	if recorder.statusCode >= 200 && recorder.statusCode < 300 {
		cached := idempotencyCachedResponse{
			StatusCode: recorder.statusCode,
			Body:       recorder.body.String(),
		}
		data, err := idempotencyJSONMarshal(cached)
		if err != nil {
			slog.Error("idempotency: failed to marshal cached response", "key_hash", redisKey, "error", err)
			return
		}
		if err := rdb.Set(r.Context(), redisKey, data, defaultIdempotencyTTL).Err(); err != nil {
			slog.Error("idempotency: failed to cache response", "key_hash", redisKey, "error", err)
		}
		return
	}
	// TODO: SETNX non-2xx delete path also needs unit test coverage.
	if err := rdb.Del(r.Context(), redisKey).Err(); err != nil {
		slog.Error("idempotency: failed to delete key", "key_hash", redisKey, "error", err)
	}
}

// SaveIdempotencyResponse stores the response in Redis for future idempotent replays.
// This is called automatically by the middleware for 2xx responses, but can also be
// called manually by handlers that need custom caching behavior.
func SaveIdempotencyResponse(ctx context.Context, rdb IdempotencyStore, key string, statusCode int, body []byte, ttl time.Duration) error {
	cached := idempotencyCachedResponse{
		StatusCode: statusCode,
		Body:       string(body),
	}
	data, err := idempotencyJSONMarshal(cached)
	if err != nil {
		return err
	}
	return rdb.Set(ctx, key, data, ttl).Err()
}

// idempotencyJSONMarshal is injectable for unit tests.
var idempotencyJSONMarshal = json.Marshal

// GetIdempotencyKey returns the Redis key for storing the idempotent response.
func GetIdempotencyKey(ctx context.Context) string {
	if key, ok := ctx.Value(idempCtxKey{}).(string); ok {
		return key
	}
	return ""
}
