package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker/v2"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/resilience"
)

// EmailWorker consumes email:queue Redis Stream and sends emails via Resend API.
type EmailWorker struct {
	rdb        *redis.Client
	apiKey     string
	from       string
	baseURL    string
	maxRetries int
	httpClient *http.Client
	cb         *gobreaker.CircuitBreaker[any]
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
		cb: resilience.NewResendBreaker(),
	}
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
func (w *EmailWorker) Start(ctx context.Context) {
	if err := w.rdb.XGroupCreateMkStream(ctx, "email:queue", "email-workers", "$").Err(); err != nil {
		slog.Debug("email worker: XGroupCreate (may already exist)", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		streams, err := w.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    "email-workers",
			Consumer: "email-worker-1",
			Streams:  []string{"email:queue", ">"},
			Count:    10,
			Block:    5 * time.Second,
		}).Result()
		if err != nil && err != redis.Nil {
			slog.Error("email worker XReadGroup", "error", err)
			time.Sleep(time.Second)
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				w.processMessage(ctx, msg)
			}
		}
	}
}

func (w *EmailWorker) processMessage(ctx context.Context, msg redis.XMessage) {
	payloadStr, ok := msg.Values["payload"].(string)
	if !ok {
		slog.Error("email worker: invalid payload", "id", msg.ID)
		w.rdb.XAck(ctx, "email:queue", "email-workers", msg.ID)
		return
	}

	var payload EmailPayload
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		slog.Error("email worker: unmarshal payload", "error", err, "id", msg.ID)
		w.rdb.XAck(ctx, "email:queue", "email-workers", msg.ID)
		return
	}

	if w.apiKey == "" {
		slog.Warn("email worker: RESEND_API_KEY not set, skipping", "to", redactEmail(payload.To))
		w.rdb.XAck(ctx, "email:queue", "email-workers", msg.ID)
		return
	}

	if err := w.sendEmail(ctx, payload); err != nil {
		w.handleSendFailure(ctx, msg, payload, err)
		return
	}

	w.rdb.XAck(ctx, "email:queue", "email-workers", msg.ID)
	slog.Info("email sent", "to", redactEmail(payload.To), "subject", payload.Subject)
}

func (w *EmailWorker) handleSendFailure(ctx context.Context, msg redis.XMessage, payload EmailPayload, sendErr error) {
	slog.Error("email worker: send failed", "error", sendErr, "to", redactEmail(payload.To))

	retryCount := 0
	if rcStr, ok := msg.Values["retry_count"].(string); ok {
		if n, err := strconv.Atoi(rcStr); err == nil {
			retryCount = n
		}
	}

	if retryCount >= w.maxRetries {
		w.rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: "email:dead-letter",
			Values: msg.Values,
		})
		w.rdb.XAck(ctx, "email:queue", "email-workers", msg.ID)
		slog.Error("email worker: moved to dead-letter after max retries", "to", redactEmail(payload.To), "retries", retryCount)
		return
	}

	msg.Values["retry_count"] = strconv.Itoa(retryCount + 1)
	w.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "email:queue",
		Values: msg.Values,
	})
	w.rdb.XAck(ctx, "email:queue", "email-workers", msg.ID)
}
