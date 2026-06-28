// Package worker runs background consumers for email and game-result queues.
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func truncateRespBody(resp *http.Response) string {
	respBody, _ := io.ReadAll(resp.Body)
	truncated := string(respBody)
	if len(truncated) > 1000 {
		truncated = truncated[:1000]
	}
	return truncated
}

// sendEmail sends a single email via the Resend HTTP API.
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
		if resp.StatusCode != http.StatusOK {
			clientErr = fmt.Errorf("resend API client error (%d): %s", resp.StatusCode, truncateRespBody(resp))
			return nil, nil
		}

		return nil, nil
	})

	if clientErr != nil {
		return clientErr
	}
	return cbErr
}
