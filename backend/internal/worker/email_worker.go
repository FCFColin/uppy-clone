package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
// 企业为何需要：异步邮件发送避免 SMTP/HTTP 延迟阻塞请求，Redis Stream 提供持久化与消费者组语义。
// 熔断器（T22）保护：Resend API 连续失败 3 次后熔断开路，避免 Worker 无谓重试堆积连接。
type EmailWorker struct {
	rdb        *redis.Client
	apiKey     string
	from       string
	baseURL    string // Resend API endpoint; defaults to "https://api.resend.com/emails". Overridable for tests.
	maxRetries int
	httpClient *http.Client // 企业为何需要：http.DefaultClient 无超时，Resend API 挂起会永久阻塞 Worker goroutine
	cb         *gobreaker.CircuitBreaker[any]
}

// NewEmailWorker creates a new EmailWorker.
// timeouts.HTTPRequestTimeout bounds total HTTP request duration;
// timeouts.HTTPConnectTimeout bounds TCP handshake.
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

// Start begins consuming the email queue. Blocks until ctx is canceled.
func (w *EmailWorker) Start(ctx context.Context) {
	// Ensure consumer group exists (ignore BUSYGROUP error if already exists)
	if err := w.rdb.XGroupCreateMkStream(ctx, "email:queue", "email-workers", "$").Err(); err != nil {
		slog.Debug("email worker: XGroupCreate (may already exist)", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Read messages
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

	// Skip if API key not configured (dev mode)
	if w.apiKey == "" {
		slog.Warn("email worker: RESEND_API_KEY not set, skipping", "to", payload.To)
		w.rdb.XAck(ctx, "email:queue", "email-workers", msg.ID)
		return
	}

	// Send email via Resend HTTP API
	if err := w.sendEmail(ctx, payload); err != nil {
		slog.Error("email worker: send failed", "error", err, "to", payload.To)

		// Determine current retry count
		retryCount := 0
		if rcStr, ok := msg.Values["retry_count"].(string); ok {
			if n, err := strconv.Atoi(rcStr); err == nil {
				retryCount = n
			}
		}

		if retryCount >= w.maxRetries {
			// Move to dead-letter queue
			w.rdb.XAdd(ctx, &redis.XAddArgs{
				Stream: "email:dead-letter",
				Values: msg.Values,
			})
			w.rdb.XAck(ctx, "email:queue", "email-workers", msg.ID)
			slog.Error("email worker: moved to dead-letter after max retries", "to", payload.To, "retries", retryCount)
			return
		}

		// Re-enqueue with incremented retry_count, then ack the original.
		// This avoids relying on XAUTOCLAIM for pending message redelivery.
		msg.Values["retry_count"] = strconv.Itoa(retryCount + 1)
		w.rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: "email:queue",
			Values: msg.Values,
		})
		w.rdb.XAck(ctx, "email:queue", "email-workers", msg.ID)
		return
	}

	w.rdb.XAck(ctx, "email:queue", "email-workers", msg.ID)
	slog.Info("email sent", "to", payload.To, "subject", payload.Subject)
}

// sendEmail sends a single email via the Resend HTTP API.
// 企业为何需要（T22）：熔断器包裹 HTTP 调用，Resend API 连续失败 3 次后开路，
// 避免无谓重试堆积连接。5xx 视为可熔断（服务端临时故障），4xx 视为不可熔断（客户端错误，重试无意义）。
func (w *EmailWorker) sendEmail(ctx context.Context, payload EmailPayload) error {
	reqBody := map[string]interface{}{
		"from":    w.from,
		"to":      []string{payload.To},
		"subject": payload.Subject,
		"html":    payload.Body,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal email request: %w", err)
	}

	// clientErr captures 4xx errors without tripping the circuit breaker.
	// gobreaker counts any non-nil callback error as failure, so we return nil
	// for 4xx and propagate via closure variable.
	var clientErr error

	_, cbErr := w.cb.Execute(func() (any, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.baseURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("create email request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+w.apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := w.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("send email: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 500 {
			respBody, _ := io.ReadAll(resp.Body)
			truncated := string(respBody)
			if len(truncated) > 1000 {
				truncated = truncated[:1000]
			}
			// 5xx: server error — return error to trip circuit breaker
			return nil, fmt.Errorf("resend API server error (%d): %s", resp.StatusCode, truncated)
		}
		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			truncated := string(respBody)
			if len(truncated) > 1000 {
				truncated = truncated[:1000]
			}
			// 4xx: client error — do NOT trip breaker, propagate via closure
			clientErr = fmt.Errorf("resend API client error (%d): %s", resp.StatusCode, truncated)
			return nil, nil
		}

		return nil, nil
	})

	if clientErr != nil {
		return clientErr
	}
	return cbErr
}
