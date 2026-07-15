// Package worker runs background consumers for email and game-result queues.
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// emailJSONMarshal is replaceable in unit tests.
var emailJSONMarshal = json.Marshal

func truncateRespBody(resp *http.Response) string {
	respBody, _ := io.ReadAll(resp.Body)
	truncated := string(respBody)
	if len(truncated) > 1000 {
		truncated = truncated[:1000]
	}
	return truncated
}

// errPermanentEmail is a sentinel for non-retryable email failures (4xx except 429).
// audit-008: The email worker previously retried all failures equally, wasting
// resources on permanent errors like invalid recipient (400) or unauthorized (401).
// Callers should check IsPermanentEmailError() and dead-letter immediately.
type errPermanentEmail struct{ inner error }

func (e *errPermanentEmail) Error() string { return e.inner.Error() }
func (e *errPermanentEmail) Unwrap() error { return e.inner }

// IsPermanentEmailError returns true for 4xx (except 429) HTTP errors that
// should not be retried.
func IsPermanentEmailError(err error) bool {
	var pe *errPermanentEmail
	return errors.As(err, &pe)
}

// sendEmail sends a single email via the Resend HTTP API.
func (w *EmailWorker) sendEmail(ctx context.Context, payload EmailPayload) error {
	reqBody := map[string]interface{}{
		"from":    w.from,
		"to":      []string{payload.To},
		"subject": payload.Subject,
		"html":    payload.Body,
	}
	bodyBytes, err := emailJSONMarshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal email request: %w", err)
	}

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
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode >= 500 {
			return nil, fmt.Errorf("resend API server error (%d): %s", resp.StatusCode, truncateRespBody(resp))
		}
		if resp.StatusCode == 429 {
			return nil, fmt.Errorf("resend API rate limited (429): %s", truncateRespBody(resp))
		}
		if resp.StatusCode >= 400 {
			// audit-008: 4xx (except 429) are permanent errors — retrying wastes resources.
			clientErr = &errPermanentEmail{inner: fmt.Errorf("resend API permanent error (%d): %s", resp.StatusCode, truncateRespBody(resp))}
			return nil, nil
		}

		return nil, nil
	})

	if clientErr != nil {
		return clientErr
	}
	return cbErr
}
