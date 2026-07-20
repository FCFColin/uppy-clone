package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker/v2"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/store"
)

// ─── Test helpers ────────────────────────────────────────────────────

// setupRedis starts a Redis testcontainer and returns a connected client.
// Skips the test if Docker is unavailable or in short mode.
func setupRedis(t *testing.T) *redis.Client {
	t.Helper()
	if testing.Short() {
		mr, err := miniredis.Run()
		if err != nil {
			t.Fatalf("miniredis: %v", err)
		}
		t.Cleanup(mr.Close)
		return redis.NewClient(&redis.Options{Addr: mr.Addr()})
	}

	ctx := context.Background()
	redisContainer, err := tcredis.Run(ctx, "redis:7-alpine",
		testcontainers.WithWaitStrategy(wait.ForLog("Ready to accept connections").WithStartupTimeout(10*time.Second)),
	)
	if err != nil {
		t.Skipf("skipping: redis container unavailable (Docker not running?): %v", err)
	}
	t.Cleanup(func() { _ = redisContainer.Terminate(ctx) })
	addr, err := redisContainer.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("failed to get redis endpoint: %v", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis ping: %v", err)
	}
	return rdb
}

// newTestWorker creates an EmailWorker connected to the given Redis and mock API URL.
func newTestWorker(rdb *redis.Client, apiURL string) *EmailWorker {
	w := NewEmailWorker(rdb, "test-api-key", "test@example.com", config.TimeoutConfig{
		HTTPRequestTimeout: 5 * time.Second,
		HTTPConnectTimeout: 3 * time.Second,
	})
	w.baseURL = apiURL
	return w
}

// makePayload creates a JSON-encoded EmailPayload string.
func makePayload(to, subject, body string) string {
	p := EmailPayload{To: to, Subject: subject, Body: body}
	b, _ := json.Marshal(p)
	return string(b)
}

func defaultTestPayload() EmailPayload {
	return EmailPayload{To: "user@test.com", Subject: "Test", Body: "body"}
}

// emailTestEnv bundles fixtures for EmailWorker.processMessage tests.
type emailTestEnv struct {
	rdb    *redis.Client
	count  *int32
	worker *EmailWorker
	ctx    context.Context
}

// setupEmailTest creates a Redis-backed EmailWorker pointed at an HTTP test server.
// handler may be nil to assert the API is never called.
func setupEmailTest(t *testing.T, handler http.HandlerFunc) *emailTestEnv {
	t.Helper()
	rdb := setupRedis(t)
	var count int32
	if handler == nil {
		handler = func(_ http.ResponseWriter, _ *http.Request) { t.Errorf("should not call API") }
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &emailTestEnv{rdb: rdb, count: &count, worker: newTestWorker(rdb, server.URL), ctx: context.Background()}
}

// sendEmailEnv bundles fixtures for EmailWorker.sendEmail tests (no Redis needed).
type sendEmailEnv struct {
	worker *EmailWorker
	count  *int32
}

func setupSendEmailTest(t *testing.T, handler http.HandlerFunc) *sendEmailEnv {
	t.Helper()
	var count int32
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &sendEmailEnv{worker: newTestWorker(nil, server.URL), count: &count}
}

func assertStreamLen(t *testing.T, rdb *redis.Client, stream string, want int64) {
	t.Helper()
	got, err := rdb.XLen(context.Background(), stream).Result()
	if err != nil {
		t.Fatalf("XLen %s: %v", stream, err)
	}
	if got != want {
		t.Fatalf("stream %s len = %d, want %d", stream, got, want)
	}
}

// assertStreamHasOneMsg returns the single message in stream (fails if != 1).
func assertStreamHasOneMsg(t *testing.T, rdb *redis.Client, stream string) redis.XMessage {
	t.Helper()
	msgs, err := rdb.XRange(context.Background(), stream, "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange %s: %v", stream, err)
	}
	if len(msgs) != 1 {
		t.Fatalf("stream %s has %d messages, want 1", stream, len(msgs))
	}
	return msgs[0]
}

// assertPayloadPreserved verifies the message's payload decodes to the expected To.
func assertPayloadPreserved(t *testing.T, msg redis.XMessage, wantTo string) {
	t.Helper()
	payload, _ := msg.Values["payload"].(string)
	var ep EmailPayload
	if err := json.Unmarshal([]byte(payload), &ep); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if ep.To != wantTo {
		t.Fatalf("expected to=%s, got %s", wantTo, ep.To)
	}
}

// runWorkerUntilCancel starts w in a goroutine and returns cancel + done.
func runWorkerUntilCancel(t *testing.T, w *EmailWorker) (cancel context.CancelFunc, done chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done = make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()
	return cancel, done
}

// waitForCancel cancels the worker and asserts Start exits within 3s.
func waitForCancel(t *testing.T, cancel context.CancelFunc, done chan struct{}) {
	t.Helper()
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not exit after cancel")
	}
}

// ─── processMessage: basic paths (success / invalid / no-api-key) ────

// TestProcessMessage_BasicPaths verifies success, invalid payloads, and missing
// API key all behave correctly (acked, queue stays empty, count matches).
func TestProcessMessage_BasicPaths(t *testing.T) {
	cases := []struct {
		name        string
		payload     interface{}
		clearAPIKey bool
		wantCount   int32
	}{
		{"success", makePayload("user@test.com", "Test", "<p>Hello</p>"), false, 1},
		{"invalid-json", "not-json", false, 0},
		{"non-string", 123, false, 0},
		{"no-api-key", makePayload("user@test.com", "Test", "body"), true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var env *emailTestEnv
			env = setupEmailTest(t, func(w http.ResponseWriter, _ *http.Request) {
				atomic.AddInt32(env.count, 1)
				w.WriteHeader(http.StatusOK)
			})
			if tc.clearAPIKey {
				env.worker.apiKey = ""
			}
			env.worker.processMessage(env.ctx, redis.XMessage{
				ID:     tc.name + "-0",
				Values: map[string]interface{}{"payload": tc.payload},
			})
			if got := atomic.LoadInt32(env.count); got != tc.wantCount {
				t.Fatalf("API call count = %d, want %d", got, tc.wantCount)
			}
			assertStreamLen(t, env.rdb, "email:queue", 0)
			assertStreamLen(t, env.rdb, "email:dead-letter", 0)
		})
	}
}

// ─── processMessage: retry logic ─────────────────────────────────────

// TestProcessMessage_Retry verifies that a failed send re-enqueues the message
// with an incremented retry_count.
func TestProcessMessage_Retry(t *testing.T) {
	var env *emailTestEnv
	env = setupEmailTest(t, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(env.count, 1)
		w.WriteHeader(http.StatusInternalServerError) // 500 → trips breaker
	})

	env.worker.processMessage(env.ctx, redis.XMessage{
		ID:     "retry-0",
		Values: map[string]interface{}{"payload": makePayload("user@test.com", "Test", "body")},
	})

	if got := atomic.LoadInt32(env.count); got != 1 {
		t.Fatalf("expected 1 API call, got %d", got)
	}
	msg := assertStreamHasOneMsg(t, env.rdb, "email:queue")
	if got := parseRetryCount(msg); got != 1 {
		t.Fatalf("retry_count = %d, want 1", got)
	}
	assertPayloadPreserved(t, msg, "user@test.com")
}

// TestProcessMessage_RetryThenSuccess verifies the full retry flow:
// first attempt fails, re-enqueued message succeeds on second attempt.
func TestProcessMessage_RetryThenSuccess(t *testing.T) {
	var env *emailTestEnv
	env = setupEmailTest(t, func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(env.count, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError) // first fails
			return
		}
		w.WriteHeader(http.StatusOK) // second succeeds
	})

	// First attempt: fails, re-enqueues with retry_count=1
	env.worker.processMessage(env.ctx, redis.XMessage{
		ID:     "retry-success-0",
		Values: map[string]interface{}{"payload": makePayload("user@test.com", "Test", "body")},
	})
	if got := atomic.LoadInt32(env.count); got != 1 {
		t.Fatalf("expected 1 API call after first attempt, got %d", got)
	}

	// Read re-enqueued message, then clear the stream so we can verify it's empty after second attempt.
	msg := assertStreamHasOneMsg(t, env.rdb, "email:queue")
	env.rdb.Del(env.ctx, "email:queue")

	// Second attempt: succeeds
	env.worker.processMessage(env.ctx, redis.XMessage{ID: msg.ID, Values: msg.Values})
	if got := atomic.LoadInt32(env.count); got != 2 {
		t.Fatalf("expected 2 API calls total, got %d", got)
	}
	assertStreamLen(t, env.rdb, "email:queue", 0)
}

// ─── processMessage: dead-letter queue ───────────────────────────────

// TestProcessMessage_DeadLetterQueue verifies that after maxRetries, the message
// is moved to the dead-letter stream instead of being re-enqueued.
func TestProcessMessage_DeadLetterQueue(t *testing.T) {
	var env *emailTestEnv
	env = setupEmailTest(t, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(env.count, 1)
		w.WriteHeader(http.StatusInternalServerError) // always fail
	})

	msg := redis.XMessage{
		ID: "deadletter-0",
		Values: map[string]interface{}{
			"payload":     makePayload("user@test.com", "Test", "body"),
			"retry_count": strconv.Itoa(env.worker.maxRetries), // at max retries
		},
	}
	env.worker.processMessage(env.ctx, msg)
	if got := atomic.LoadInt32(env.count); got != 1 {
		t.Fatalf("expected 1 API call, got %d", got)
	}

	// Message should be in dead-letter, NOT re-enqueued
	dlMsg := assertStreamHasOneMsg(t, env.rdb, "email:dead-letter")
	assertPayloadPreserved(t, dlMsg, "user@test.com")
	assertStreamLen(t, env.rdb, "email:queue", 0)
}

// ─── sendEmail: HTTP server fixture ──────────────────────────────────

// TestSendEmail_TableDriven verifies success, 4xx (breaker stays closed), and
// 5xx with long body (error is truncated) in a single table-driven test.
func TestSendEmail_TableDriven(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		body        string
		wantErr     bool
		wantBreaker gobreaker.State
	}{
		{"success", http.StatusOK, "", false, gobreaker.StateClosed},
		{"client-4xx-keeps-closed", http.StatusBadRequest, "", true, gobreaker.StateClosed},
		{"server-5xx-truncates-body", http.StatusInternalServerError, strings.Repeat("x", 1500), true, gobreaker.StateClosed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() { store.ResetBreakersForTesting() })
			var env *sendEmailEnv
			env = setupSendEmailTest(t, func(w http.ResponseWriter, _ *http.Request) {
				atomic.AddInt32(env.count, 1)
				w.WriteHeader(tc.status)
				if tc.body != "" {
					_, _ = w.Write([]byte(tc.body))
				}
			})
			err := env.worker.sendEmail(context.Background(), defaultTestPayload())
			if (err != nil) != tc.wantErr {
				t.Fatalf("sendEmail err = %v, wantErr %v", err, tc.wantErr)
			}
			if env.worker.cb.State() != tc.wantBreaker {
				t.Fatalf("breaker state = %v, want %v", env.worker.cb.State(), tc.wantBreaker)
			}
			if tc.body != "" && err != nil && len(err.Error()) > 1100 {
				t.Fatalf("expected truncated error body, got len=%d", len(err.Error()))
			}
		})
	}
}

// TestSendEmail_CircuitBreakerOpens verifies that after 4 consecutive 5xx errors
// (ConsecutiveFailures > 3), the circuit breaker opens and subsequent calls
// return ErrOpenState without hitting the API.
func TestSendEmail_CircuitBreakerOpens(t *testing.T) {
	t.Cleanup(func() { store.ResetBreakersForTesting() })
	var env *sendEmailEnv
	env = setupSendEmailTest(t, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(env.count, 1)
		w.WriteHeader(http.StatusInternalServerError)
	})
	payload := defaultTestPayload()
	ctx := context.Background()

	// ResendBreaker trips after ConsecutiveFailures > 3 (i.e., 4 failures)
	for i := 0; i < 4; i++ {
		if err := env.worker.sendEmail(ctx, payload); err == nil {
			t.Fatalf("expected error on attempt %d", i+1)
		}
	}
	if env.worker.cb.State() != gobreaker.StateOpen {
		t.Fatalf("expected breaker to be open after 4 failures, got %v", env.worker.cb.State())
	}

	// Subsequent call should fail immediately without hitting the API
	beforeCount := atomic.LoadInt32(env.count)
	if err := env.worker.sendEmail(ctx, payload); err == nil {
		t.Fatal("expected error when breaker is open")
	}
	if afterCount := atomic.LoadInt32(env.count); afterCount != beforeCount {
		t.Fatalf("expected no API call when breaker is open, got %d additional calls", afterCount-beforeCount)
	}
}

// TestSendEmail_AuthorizationHeader verifies the API key is sent in the Authorization header.
func TestSendEmail_AuthorizationHeader(t *testing.T) {
	var authHeader string
	env := setupSendEmailTest(t, func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	})
	env.worker.apiKey = "my-secret-key"
	if err := env.worker.sendEmail(context.Background(), defaultTestPayload()); err != nil {
		t.Fatalf("sendEmail: %v", err)
	}
	if authHeader != "Bearer my-secret-key" {
		t.Fatalf("expected Authorization 'Bearer my-secret-key', got %q", authHeader)
	}
}

type errRoundTripper struct{ err error }

func (e errRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, e.err
}

// TestSendEmail_ErrorPaths verifies sendEmail surfaces errors for bad URLs and
// network failures.
func TestSendEmail_ErrorPaths(t *testing.T) {
	t.Run("network-error", func(t *testing.T) {
		w := newTestWorker(nil, "http://example.com/emails")
		w.httpClient = &http.Client{Transport: errRoundTripper{err: errors.New("network down")}}
		if err := w.sendEmail(context.Background(), defaultTestPayload()); err == nil {
			t.Fatal("expected network error")
		}
	})
	t.Run("invalid-url", func(t *testing.T) {
		w := newTestWorker(nil, "://invalid-url")
		if err := w.sendEmail(context.Background(), defaultTestPayload()); err == nil {
			t.Fatal("expected request creation error")
		}
	})
}

// ─── Start lifecycle ─────────────────────────────────────────────────

func TestEmailWorker_Start_XReadGroupError(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	w := newTestWorker(rdb, "http://127.0.0.1:1")

	cancel, done := runWorkerUntilCancel(t, w)
	time.Sleep(50 * time.Millisecond)
	mr.Close()
	time.Sleep(1100 * time.Millisecond)
	waitForCancel(t, cancel, done)
}

func TestEmailWorker_Start_Cancelled(t *testing.T) {
	rdb := setupRedis(t)
	w := newTestWorker(rdb, "http://localhost")
	cancel, done := runWorkerUntilCancel(t, w)
	waitForCancel(t, cancel, done)
}

func TestEmailWorker_Start_ProcessesMessage(t *testing.T) {
	var env *emailTestEnv
	env = setupEmailTest(t, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(env.count, 1)
		w.WriteHeader(http.StatusOK)
	})
	if _, err := env.rdb.XAdd(env.ctx, &redis.XAddArgs{
		Stream: "email:queue",
		Values: map[string]interface{}{"payload": makePayload("user@test.com", "Start test", "body")},
	}).Result(); err != nil {
		t.Fatalf("XAdd: %v", err)
	}
	if err := env.rdb.XGroupCreateMkStream(env.ctx, "email:queue", "email-workers", "0").Err(); err != nil {
		t.Fatalf("XGroupCreateMkStream: %v", err)
	}

	cancel, done := runWorkerUntilCancel(t, env.worker)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(env.count) >= 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := atomic.LoadInt32(env.count); got != 1 {
		t.Fatalf("expected 1 API call from Start, got %d", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(6 * time.Second):
		t.Fatal("Start did not exit after cancel")
	}
}

func TestTruncateRespBody_TruncatesLongBody(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(strings.Repeat("x", 1500)))}
	if got := truncateRespBody(resp); len(got) != 1000 {
		t.Fatalf("len = %d, want 1000", len(got))
	}
}

func TestNewGDPRCleanupWorker(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		retentionDays int
		interval      time.Duration
		wantRetention int
		wantInterval  time.Duration
	}{
		{"defaults", 0, 0, defaultGDPRRetentionDays, defaultGDPRCleanupInterval},
		{"custom", 60, 12 * time.Hour, 60, 12 * time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := NewGDPRCleanupWorker(nil, tc.retentionDays, tc.interval)
			if w.retentionDays != tc.wantRetention {
				t.Errorf("retentionDays = %d, want %d", w.retentionDays, tc.wantRetention)
			}
			if w.interval != tc.wantInterval {
				t.Errorf("interval = %v, want %v", w.interval, tc.wantInterval)
			}
		})
	}
}
