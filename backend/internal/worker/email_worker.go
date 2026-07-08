package worker

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker/v2"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/resilience"
	"github.com/uppy-clone/backend/internal/slogctx"
)

// emailWorkerName is the metrics label value for the email worker.
const emailWorkerName = "email"

// EmailWorker consumes email:queue Redis Stream and sends emails via Resend API.
type EmailWorker struct {
	rdb        *redis.Client
	apiKey     string
	from       string
	baseURL    string
	maxRetries int
	httpClient *http.Client
	cb         *gobreaker.CircuitBreaker[any]
	consumerID string
}

// NewEmailWorker creates a new EmailWorker.
func NewEmailWorker(rdb *redis.Client, apiKey, from string, timeouts config.TimeoutConfig) *EmailWorker {
	return &EmailWorker{
		rdb:        rdb,
		apiKey:     apiKey,
		from:       from,
		baseURL:    "https://api.resend.com/emails",
		maxRetries: 5,
		httpClient: &http.Client{
			Timeout: timeouts.HTTPRequestTimeout,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout: timeouts.HTTPConnectTimeout,
				}).DialContext,
			},
		},
		cb:         resilience.NewResendBreaker(),
		consumerID: resolveConsumerID("email-worker"),
	}
}

// resolveConsumerID returns a consumer identifier for Redis Stream groups.
// Precedence (v2-R-42): EMAIL_WORKER_CONSUMER_ID env > HOSTNAME env (pod name in K8s)
// > os.Hostname() > fallback. This avoids hardcoding "email-worker-1" which causes
// multi-instance consumers to share a single consumer entry (breaking load distribution
// and dead-letter attribution).
func resolveConsumerID(prefix string) string {
	if v := os.Getenv("EMAIL_WORKER_CONSUMER_ID"); v != "" {
		return v
	}
	if h := os.Getenv("HOSTNAME"); h != "" {
		return prefix + "-" + h
	}
	if h, err := os.Hostname(); err == nil && h != "" {
		return prefix + "-" + h
	}
	return prefix + "-1"
}

// EmailPayload is the message format enqueued by the application.
type EmailPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func redactEmail(email string) string {
	if len(email) <= 3 {
		return "***"
	}
	return email[:3] + "***"
}

// Start begins consuming the email queue. Blocks until ctx is canceled.
//
// Backoff (v2-R-43): on XReadGroup errors, sleep with exponential backoff
// (capped at maxReadBackoff) to avoid hammering Redis when it is degraded.
func (w *EmailWorker) Start(ctx context.Context) {
	logger := slogctx.LoggerFromContext(ctx).With("worker", emailWorkerName, "consumer", w.consumerID)
	ctx = slogctx.WithLogger(ctx, logger)

	if err := w.rdb.XGroupCreateMkStream(ctx, "email:queue", "email-workers", "$").Err(); err != nil {
		logger.Debug("email worker: XGroupCreate (may already exist)", "error", err)
	}

	const (
		initialBackoff = 100 * time.Millisecond
		maxBackoff     = 10 * time.Second
	)
	backoff := initialBackoff

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		streams, err := w.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    "email-workers",
			Consumer: w.consumerID,
			Streams:  []string{"email:queue", ">"},
			Count:    10,
			Block:    5 * time.Second,
		}).Result()
		if err != nil && err != redis.Nil {
			logger.Error("email worker XReadGroup", "error", err)
			metrics.WorkerReadErrors.WithLabelValues(emailWorkerName).Inc()
			// Exponential backoff: double the delay each consecutive error, cap at maxBackoff.
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		// Reset backoff after a successful read (including redis.Nil which means no messages).
		backoff = initialBackoff

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				w.processMessage(ctx, msg)
			}
		}
	}
}

func (w *EmailWorker) processMessage(ctx context.Context, msg redis.XMessage) {
	start := time.Now()
	logger := slogctx.LoggerFromContext(ctx)

	payloadStr, ok := msg.Values["payload"].(string)
	if !ok {
		logger.Error("email worker: invalid payload", "id", msg.ID)
		w.rdb.XAck(ctx, "email:queue", "email-workers", msg.ID)
		metrics.WorkerMessagesProcessed.WithLabelValues(emailWorkerName, "invalid_payload").Inc()
		metrics.WorkerProcessingDuration.WithLabelValues(emailWorkerName).Observe(time.Since(start).Seconds())
		return
	}

	var payload EmailPayload
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		logger.Error("email worker: unmarshal payload", "error", err, "id", msg.ID)
		w.rdb.XAck(ctx, "email:queue", "email-workers", msg.ID)
		metrics.WorkerMessagesProcessed.WithLabelValues(emailWorkerName, "invalid_payload").Inc()
		metrics.WorkerProcessingDuration.WithLabelValues(emailWorkerName).Observe(time.Since(start).Seconds())
		return
	}

	if w.apiKey == "" {
		logger.Warn("email worker: RESEND_API_KEY not set, skipping", "to", redactEmail(payload.To))
		w.rdb.XAck(ctx, "email:queue", "email-workers", msg.ID)
		metrics.WorkerMessagesProcessed.WithLabelValues(emailWorkerName, "skipped").Inc()
		metrics.WorkerProcessingDuration.WithLabelValues(emailWorkerName).Observe(time.Since(start).Seconds())
		return
	}

	if err := w.sendEmail(ctx, payload); err != nil {
		w.handleSendFailure(ctx, msg, payload, err)
		metrics.WorkerMessagesProcessed.WithLabelValues(emailWorkerName, "failure").Inc()
		metrics.WorkerProcessingDuration.WithLabelValues(emailWorkerName).Observe(time.Since(start).Seconds())
		return
	}

	w.rdb.XAck(ctx, "email:queue", "email-workers", msg.ID)
	logger.Info("email sent", "to", redactEmail(payload.To), "subject", payload.Subject)
	metrics.WorkerMessagesProcessed.WithLabelValues(emailWorkerName, "success").Inc()
	metrics.WorkerProcessingDuration.WithLabelValues(emailWorkerName).Observe(time.Since(start).Seconds())
}

func (w *EmailWorker) handleSendFailure(ctx context.Context, msg redis.XMessage, payload EmailPayload, sendErr error) {
	logger := slogctx.LoggerFromContext(ctx)
	logger.Error("email worker: send failed", "error", sendErr, "to", redactEmail(payload.To))
	// handleRetry re-enqueues with exponential backoff (100ms * 2^retryCount) up to
	// maxRetries, then moves to dead-letter stream. See retry.go.
	deadLettered := handleRetry(ctx, w.rdb, msg, "email:queue", "email-workers", "email:dead-letter", w.maxRetries, "worker", "email", "to", redactEmail(payload.To))
	if deadLettered {
		metrics.WorkerMessagesProcessed.WithLabelValues(emailWorkerName, "deadletter").Inc()
	}
}
