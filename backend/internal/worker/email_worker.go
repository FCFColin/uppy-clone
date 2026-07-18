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
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/util"
)

// emailWorkerName is the metrics label value for the email worker.
const emailWorkerName = "email"

// EmailWorker consumes email:queue Redis Stream and sends emails via Resend API.
type EmailWorker struct {
	rdb        RedisStreamConsumer
	apiKey     string
	from       string
	baseURL    string
	maxRetries int
	httpClient *http.Client
	cb         *gobreaker.CircuitBreaker[any]
	consumerID string
}

// NewEmailWorker creates a new EmailWorker.
func NewEmailWorker(rdb RedisStreamConsumer, apiKey, from string, timeouts config.TimeoutConfig) *EmailWorker {
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
		cb:         store.NewResendBreaker(),
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
//
// XAUTOCLAIM (audit project-04-001): a background goroutine periodically
// reclaims messages stuck in the PEL (Pending Entries List) of consumers
// that crashed or became unresponsive, ensuring at-least-once delivery.
func (w *EmailWorker) Start(ctx context.Context) {
	logger := util.LoggerFromContext(ctx).With("worker", emailWorkerName, "consumer", w.consumerID)
	ctx = util.WithLogger(ctx, logger)

	if err := w.rdb.XGroupCreateMkStream(ctx, emailQueueStream, emailWorkersGroup, "$").Err(); err != nil {
		// audit-023: Upgrade from Debug to Warn — a failure here (other than "BUSYGROUP")
		// means the worker cannot consume messages, which is operationally significant.
		// XGroupCreateMkStream creates the stream if it doesn't exist, so most errors
		// are unexpected. The common "BUSYGROUP" (group already exists) is expected
		// during restarts and is handled by the MkStream semantics.
		logger.Warn("email worker: XGroupCreate failed (may already exist)", "error", err)
	}

	// Start XAUTOCLAIM background goroutine to reclaim zombie consumer messages.
	go w.claimPendingMessages(ctx)

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
			Group:    emailWorkersGroup,
			Consumer: w.consumerID,
			Streams:  []string{emailQueueStream, ">"},
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

// claimPendingMessages periodically runs XAUTOCLAIM to reclaim messages
// stuck in the PEL of consumers that crashed or became unresponsive.
// This ensures at-least-once delivery as required by ADR-007/009/010.
func (w *EmailWorker) claimPendingMessages(ctx context.Context) {
	runClaimPendingMessages(ctx, w.rdb, w.consumerID, claimLoopConfig{
		stream:     emailQueueStream,
		group:      emailWorkersGroup,
		workerName: "email worker",
	}, w.processMessage)
}

func (w *EmailWorker) processMessage(ctx context.Context, msg redis.XMessage) {
	start := time.Now()
	logger := util.LoggerFromContext(ctx)

	payloadStr, ok := msg.Values["payload"].(string) //nolint:goconst // Redis stream field name
	if !ok {
		logger.Error("email worker: invalid payload", "id", msg.ID)
		if ackErr := w.rdb.XAck(ctx, emailQueueStream, emailWorkersGroup, msg.ID).Err(); ackErr != nil {
			logger.Error("failed to ack invalid payload", "error", ackErr, "id", msg.ID)
			metrics.WorkerAckErrors.WithLabelValues(emailWorkerName).Inc()
		}
		metrics.WorkerMessagesProcessed.WithLabelValues(emailWorkerName, "invalid_payload").Inc()
		metrics.WorkerProcessingDuration.WithLabelValues(emailWorkerName).Observe(time.Since(start).Seconds())
		return
	}

	var payload EmailPayload
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		logger.Error("email worker: unmarshal payload", "error", err, "id", msg.ID)
		if ackErr := w.rdb.XAck(ctx, emailQueueStream, emailWorkersGroup, msg.ID).Err(); ackErr != nil {
			logger.Error("failed to ack unmarshal error", "error", ackErr, "id", msg.ID)
			metrics.WorkerAckErrors.WithLabelValues(emailWorkerName).Inc()
		}
		metrics.WorkerMessagesProcessed.WithLabelValues(emailWorkerName, "invalid_payload").Inc()
		metrics.WorkerProcessingDuration.WithLabelValues(emailWorkerName).Observe(time.Since(start).Seconds())
		return
	}

	if w.apiKey == "" {
		// audit-004: Do NOT XAck when apiKey is missing — the message stays in the
		// PEL and XAUTOCLAIM will reclaim it later. Acks here would silently drop
		// the email, violating at-least-once delivery (ADR-007/009/010).
		logger.Error("email worker: RESEND_API_KEY not set, leaving message in PEL for retry",
			"to", redactEmail(payload.To), "id", msg.ID)
		metrics.WorkerMessagesProcessed.WithLabelValues(emailWorkerName, "no_apikey_pending").Inc()
		metrics.WorkerProcessingDuration.WithLabelValues(emailWorkerName).Observe(time.Since(start).Seconds())
		return
	}

	if err := w.sendEmail(ctx, payload); err != nil {
		w.handleSendFailure(ctx, msg, payload, err)
		metrics.WorkerMessagesProcessed.WithLabelValues(emailWorkerName, "failure").Inc()
		metrics.WorkerProcessingDuration.WithLabelValues(emailWorkerName).Observe(time.Since(start).Seconds())
		return
	}

	if ackErr := w.rdb.XAck(ctx, emailQueueStream, emailWorkersGroup, msg.ID).Err(); ackErr != nil {
		logger.Error("failed to ack sent message", "error", ackErr, "id", msg.ID)
		metrics.WorkerAckErrors.WithLabelValues(emailWorkerName).Inc()
	}
	logger.Info("email sent", "to", redactEmail(payload.To), "subject", payload.Subject)
	metrics.WorkerMessagesProcessed.WithLabelValues(emailWorkerName, "success").Inc()
	metrics.WorkerProcessingDuration.WithLabelValues(emailWorkerName).Observe(time.Since(start).Seconds())
}

func (w *EmailWorker) handleSendFailure(ctx context.Context, msg redis.XMessage, payload EmailPayload, sendErr error) {
	logger := util.LoggerFromContext(ctx)
	logger.Error("email worker: send failed", "error", sendErr, "to", redactEmail(payload.To))

	// audit-008: Permanent errors (4xx except 429) should not be retried.
	// Move directly to dead-letter queue to avoid wasting retry cycles.
	if IsPermanentEmailError(sendErr) {
		logger.Warn("email worker: permanent error, moving to dead-letter", "id", msg.ID, "error", sendErr)
		if dlErr := w.rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: "email:dead-letter",
			MaxLen: 10000,
			Approx: true,
			Values: map[string]interface{}{
				"payload":         msg.Values["payload"], //nolint:goconst // Redis stream field name
				"error":           sendErr.Error(),
				"reason":          "permanent_error",
				"orig_id":         msg.ID,
				"to":              redactEmail(payload.To),
				"deadlettered_at": time.Now().UnixMilli(),
			},
		}).Err(); dlErr != nil {
			logger.Error("failed to add to dead-letter stream", "error", dlErr, "id", msg.ID)
		}
		if ackErr := w.rdb.XAck(ctx, emailQueueStream, emailWorkersGroup, msg.ID).Err(); ackErr != nil {
			logger.Error("failed to ack permanent-error message", "error", ackErr, "id", msg.ID)
			metrics.WorkerAckErrors.WithLabelValues(emailWorkerName).Inc()
		}
		metrics.WorkerMessagesProcessed.WithLabelValues(emailWorkerName, "deadletter_permanent").Inc()
		return
	}

	// Transient errors (5xx, 429, network): retry with exponential backoff.
	deadLettered := handleRetry(ctx, w.rdb, msg, emailQueueStream, emailWorkersGroup, "email:dead-letter", w.maxRetries, "worker", "email", "to", redactEmail(payload.To))
	if deadLettered {
		metrics.WorkerMessagesProcessed.WithLabelValues(emailWorkerName, "deadletter").Inc()
	}
}
