package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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
	redisContainer, err := tcredis.Run(ctx,
		"redis:7-alpine",
		testcontainers.WithWaitStrategy(
			wait.ForLog("Ready to accept connections").
				WithStartupTimeout(10*time.Second)),
	)
	if err != nil {
		t.Skipf("skipping: redis container unavailable (Docker not running?): %v", err)
	}
	t.Cleanup(func() { redisContainer.Terminate(ctx) })

	addr, err := redisContainer.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("failed to get redis endpoint: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { rdb.Close() })

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

// ─── processMessage: success ─────────────────────────────────────────

// TestProcessMessage_Success verifies that a valid message is processed,
// email is sent, and the message is acked.
func TestProcessMessage_Success(t *testing.T) {
	rdb := setupRedis(t)

	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	w := newTestWorker(rdb, server.URL)
	ctx := context.Background()

	msg := redis.XMessage{
		ID:     "1234567890-0",
		Values: map[string]interface{}{"payload": makePayload("user@test.com", "Test", "<p>Hello</p>")},
	}
	w.processMessage(ctx, msg)

	if atomic.LoadInt32(&requestCount) != 1 {
		t.Fatalf("expected 1 API call, got %d", atomic.LoadInt32(&requestCount))
	}

	// Verify no re-enqueued messages in email:queue
	queueLen, err := rdb.XLen(ctx, "email:queue").Result()
	if err != nil {
		t.Fatalf("XLen email:queue: %v", err)
	}
	if queueLen != 0 {
		t.Fatalf("expected 0 messages in email:queue, got %d", queueLen)
	}

	// Verify no dead-letter messages
	dlLen, err := rdb.XLen(ctx, "email:dead-letter").Result()
	if err != nil {
		t.Fatalf("XLen email:dead-letter: %v", err)
	}
	if dlLen != 0 {
		t.Fatalf("expected 0 messages in dead-letter, got %d", dlLen)
	}
}

// TestProcessMessage_InvalidPayload verifies that a message with invalid payload
// is acked without sending an email.
func TestProcessMessage_InvalidPayload(t *testing.T) {
	rdb := setupRedis(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not call API for invalid payload")
	}))
	defer server.Close()

	w := newTestWorker(rdb, server.URL)
	ctx := context.Background()

	msg := redis.XMessage{
		ID:     "invalid-0",
		Values: map[string]interface{}{"payload": "not-json"},
	}
	w.processMessage(ctx, msg)

	// No re-enqueue, no dead-letter
	queueLen, _ := rdb.XLen(ctx, "email:queue").Result()
	if queueLen != 0 {
		t.Fatalf("expected 0 messages in email:queue, got %d", queueLen)
	}
}

// TestProcessMessage_NoAPIKey verifies that when apiKey is empty, the message
// is acked without sending (dev mode).
func TestProcessMessage_NoAPIKey(t *testing.T) {
	rdb := setupRedis(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not call API when apiKey is empty")
	}))
	defer server.Close()

	w := newTestWorker(rdb, server.URL)
	w.apiKey = "" // simulate dev mode
	ctx := context.Background()

	msg := redis.XMessage{
		ID:     "nokey-0",
		Values: map[string]interface{}{"payload": makePayload("user@test.com", "Test", "body")},
	}
	w.processMessage(ctx, msg)

	// No re-enqueue
	queueLen, _ := rdb.XLen(ctx, "email:queue").Result()
	if queueLen != 0 {
		t.Fatalf("expected 0 messages in email:queue, got %d", queueLen)
	}
}

// ─── processMessage: retry logic ─────────────────────────────────────

// TestProcessMessage_Retry verifies that a failed send re-enqueues the message
// with an incremented retry_count.
func TestProcessMessage_Retry(t *testing.T) {
	rdb := setupRedis(t)

	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusInternalServerError) // 500 → trips breaker
	}))
	defer server.Close()

	w := newTestWorker(rdb, server.URL)
	ctx := context.Background()

	msg := redis.XMessage{
		ID:     "retry-0",
		Values: map[string]interface{}{"payload": makePayload("user@test.com", "Test", "body")},
	}
	w.processMessage(ctx, msg)

	if atomic.LoadInt32(&requestCount) != 1 {
		t.Fatalf("expected 1 API call, got %d", atomic.LoadInt32(&requestCount))
	}

	// Message should be re-enqueued with retry_count=1
	msgs, err := rdb.XRange(ctx, "email:queue", "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 re-enqueued message, got %d", len(msgs))
	}

	retryStr, ok := msgs[0].Values["retry_count"].(string)
	if !ok {
		t.Fatal("re-enqueued message missing retry_count")
	}
	retryCount, err := strconv.Atoi(retryStr)
	if err != nil {
		t.Fatalf("invalid retry_count: %v", err)
	}
	if retryCount != 1 {
		t.Fatalf("expected retry_count=1, got %d", retryCount)
	}

	// Verify payload is preserved
	payload, ok := msgs[0].Values["payload"].(string)
	if !ok {
		t.Fatal("re-enqueued message missing payload")
	}
	var ep EmailPayload
	if err := json.Unmarshal([]byte(payload), &ep); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if ep.To != "user@test.com" {
		t.Fatalf("expected to=user@test.com, got %s", ep.To)
	}
}

// TestProcessMessage_RetryThenSuccess verifies the full retry flow:
// first attempt fails, re-enqueued message succeeds on second attempt.
func TestProcessMessage_RetryThenSuccess(t *testing.T) {
	rdb := setupRedis(t)

	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&requestCount, 1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError) // first fails
			return
		}
		w.WriteHeader(http.StatusOK) // second succeeds
	}))
	defer server.Close()

	w := newTestWorker(rdb, server.URL)
	ctx := context.Background()

	// First attempt: fails, re-enqueues with retry_count=1
	msg1 := redis.XMessage{
		ID:     "retry-success-0",
		Values: map[string]interface{}{"payload": makePayload("user@test.com", "Test", "body")},
	}
	w.processMessage(ctx, msg1)

	if atomic.LoadInt32(&requestCount) != 1 {
		t.Fatalf("expected 1 API call after first attempt, got %d", atomic.LoadInt32(&requestCount))
	}

	// Read re-enqueued message
	msgs, err := rdb.XRange(ctx, "email:queue", "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 re-enqueued message, got %d", len(msgs))
	}

	// Delete the stream entry so we can check it's empty after second attempt
	rdb.Del(ctx, "email:queue")

	// Second attempt: succeeds
	msg2 := redis.XMessage{
		ID:     msgs[0].ID,
		Values: msgs[0].Values,
	}
	w.processMessage(ctx, msg2)

	if atomic.LoadInt32(&requestCount) != 2 {
		t.Fatalf("expected 2 API calls total, got %d", atomic.LoadInt32(&requestCount))
	}

	// No more re-enqueued messages
	queueLen, _ := rdb.XLen(ctx, "email:queue").Result()
	if queueLen != 0 {
		t.Fatalf("expected 0 messages in email:queue after success, got %d", queueLen)
	}
}

// ─── processMessage: dead-letter queue ───────────────────────────────

// TestProcessMessage_DeadLetterQueue verifies that after maxRetries, the message
// is moved to the dead-letter stream instead of being re-enqueued.
func TestProcessMessage_DeadLetterQueue(t *testing.T) {
	rdb := setupRedis(t)

	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusInternalServerError) // always fail
	}))
	defer server.Close()

	w := newTestWorker(rdb, server.URL)
	// maxRetries is 5 by default; set retry_count to maxRetries to trigger dead-letter
	ctx := context.Background()

	msg := redis.XMessage{
		ID: "deadletter-0",
		Values: map[string]interface{}{
			"payload":     makePayload("user@test.com", "Test", "body"),
			"retry_count": strconv.Itoa(w.maxRetries), // at max retries
		},
	}
	w.processMessage(ctx, msg)

	if atomic.LoadInt32(&requestCount) != 1 {
		t.Fatalf("expected 1 API call, got %d", atomic.LoadInt32(&requestCount))
	}

	// Message should be in dead-letter, NOT re-enqueued
	dlMsgs, err := rdb.XRange(ctx, "email:dead-letter", "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange dead-letter: %v", err)
	}
	if len(dlMsgs) != 1 {
		t.Fatalf("expected 1 dead-letter message, got %d", len(dlMsgs))
	}

	// Verify payload preserved in dead-letter
	payload, ok := dlMsgs[0].Values["payload"].(string)
	if !ok {
		t.Fatal("dead-letter message missing payload")
	}
	var ep EmailPayload
	if err := json.Unmarshal([]byte(payload), &ep); err != nil {
		t.Fatalf("unmarshal dead-letter payload: %v", err)
	}
	if ep.To != "user@test.com" {
		t.Fatalf("expected to=user@test.com, got %s", ep.To)
	}

	// No re-enqueued message in email:queue
	queueLen, _ := rdb.XLen(ctx, "email:queue").Result()
	if queueLen != 0 {
		t.Fatalf("expected 0 messages in email:queue, got %d", queueLen)
	}
}

// ─── sendEmail: circuit breaker (no Redis required) ──────────────────

// TestSendEmail_CircuitBreakerOpens verifies that after 4 consecutive 5xx errors
// (ConsecutiveFailures > 3), the circuit breaker opens and subsequent calls
// return ErrOpenState without hitting the API.
func TestSendEmail_CircuitBreakerOpens(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusInternalServerError) // 5xx trips breaker
	}))
	defer server.Close()

	// sendEmail doesn't use Redis — nil is safe for these tests.
	w := newTestWorker(nil, server.URL)
	ctx := context.Background()
	payload := EmailPayload{To: "user@test.com", Subject: "Test", Body: "body"}

	// ResendBreaker trips after ConsecutiveFailures > 3 (i.e., 4 failures)
	for i := 0; i < 4; i++ {
		err := w.sendEmail(ctx, payload)
		if err == nil {
			t.Fatalf("expected error on attempt %d", i+1)
		}
	}

	// Breaker should now be open
	if w.cb.State() != gobreaker.StateOpen {
		t.Fatalf("expected breaker to be open after 4 failures, got %v", w.cb.State())
	}

	// Subsequent call should fail immediately without hitting the API
	beforeCount := atomic.LoadInt32(&requestCount)
	err := w.sendEmail(ctx, payload)
	if err == nil {
		t.Fatal("expected error when breaker is open")
	}
	afterCount := atomic.LoadInt32(&requestCount)
	if afterCount != beforeCount {
		t.Fatalf("expected no API call when breaker is open, got %d additional calls",
			afterCount-beforeCount)
	}
}

// TestSendEmail_CircuitBreakerStaysClosedOn4xx verifies that 4xx errors do NOT
// trip the circuit breaker (client errors are not retryable).
func TestSendEmail_CircuitBreakerStaysClosedOn4xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest) // 4xx → clientErr, does NOT trip breaker
	}))
	defer server.Close()

	w := newTestWorker(nil, server.URL)
	ctx := context.Background()
	payload := EmailPayload{To: "user@test.com", Subject: "Test", Body: "body"}

	// Make 5 calls with 4xx — breaker should stay closed
	for i := 0; i < 5; i++ {
		err := w.sendEmail(ctx, payload)
		if err == nil {
			t.Fatalf("expected client error on attempt %d", i+1)
		}
	}

	if w.cb.State() != gobreaker.StateClosed {
		t.Fatalf("expected breaker to stay closed after 4xx errors, got %v", w.cb.State())
	}
}

// TestSendEmail_Success verifies a successful API call returns nil.
func TestSendEmail_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	w := newTestWorker(nil, server.URL)
	ctx := context.Background()
	payload := EmailPayload{To: "user@test.com", Subject: "Test", Body: "body"}

	if err := w.sendEmail(ctx, payload); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if w.cb.State() != gobreaker.StateClosed {
		t.Fatalf("expected breaker to stay closed after success, got %v", w.cb.State())
	}
}

// TestSendEmail_AuthorizationHeader verifies the API key is sent in the Authorization header.
func TestSendEmail_AuthorizationHeader(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	w := newTestWorker(nil, server.URL)
	w.apiKey = "my-secret-key"
	ctx := context.Background()
	payload := EmailPayload{To: "user@test.com", Subject: "Test", Body: "body"}

	if err := w.sendEmail(ctx, payload); err != nil {
		t.Fatalf("sendEmail: %v", err)
	}

	if authHeader != "Bearer my-secret-key" {
		t.Fatalf("expected Authorization 'Bearer my-secret-key', got %q", authHeader)
	}
}

func TestNewGDPRCleanupWorker_Defaults(t *testing.T) {
	t.Parallel()
	w := NewGDPRCleanupWorker(nil, 0, 0)
	if w.retentionDays != defaultGDPRRetentionDays {
		t.Errorf("retentionDays = %d, want %d", w.retentionDays, defaultGDPRRetentionDays)
	}
	if w.interval != defaultGDPRCleanupInterval {
		t.Errorf("interval = %v, want %v", w.interval, defaultGDPRCleanupInterval)
	}
}

func TestNewGDPRCleanupWorker_CustomValues(t *testing.T) {
	t.Parallel()
	w := NewGDPRCleanupWorker(nil, 60, 12*time.Hour)
	if w.retentionDays != 60 || w.interval != 12*time.Hour {
		t.Errorf("unexpected gdpr worker config: %+v", w)
	}
}

func TestEmailWorker_Start_Cancelled(t *testing.T) {
	rdb := setupRedis(t)
	w := newTestWorker(rdb, "http://localhost")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not exit after cancel")
	}
}

func TestEmailWorker_Start_ProcessesMessage(t *testing.T) {
	rdb := setupRedis(t)
	ctx := context.Background()

	if _, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "email:queue",
		Values: map[string]interface{}{
			"payload": makePayload("user@test.com", "Start test", "body"),
		},
	}).Result(); err != nil {
		t.Fatalf("XAdd: %v", err)
	}
	if err := rdb.XGroupCreateMkStream(ctx, "email:queue", "email-workers", "0").Err(); err != nil {
		t.Fatalf("XGroupCreateMkStream: %v", err)
	}

	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	w := newTestWorker(rdb, server.URL)
	workerCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Start(workerCtx)
		close(done)
	}()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&requestCount) >= 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if atomic.LoadInt32(&requestCount) != 1 {
		t.Fatalf("expected 1 API call from Start, got %d", atomic.LoadInt32(&requestCount))
	}

	cancel()
	select {
	case <-done:
	case <-time.After(6 * time.Second):
		t.Fatal("Start did not exit after cancel")
	}
}
