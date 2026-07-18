// Package apierror provides RFC 7807 problem details for HTTP API errors.
package apierror

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

// ProblemDetails represents an RFC 7807 problem details response.
type ProblemDetails struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

// New constructs a ProblemDetails with the given status, title, and detail.
func New(status int, title, detail string) *ProblemDetails {
	if status < 100 || status >= 600 {
		status = http.StatusInternalServerError
		title = "Internal Server Error"
	}
	return &ProblemDetails{
		Type:   fmt.Sprintf("https://httpstatuses.com/%d", status),
		Title:  title,
		Status: status,
		Detail: detail,
	}
}

// BadRequest returns a 400 problem details response.
func BadRequest(detail string) *ProblemDetails {
	return New(http.StatusBadRequest, "Bad Request", detail)
}

// Unauthorized returns a 401 problem details response.
func Unauthorized(detail string) *ProblemDetails {
	return New(http.StatusUnauthorized, "Unauthorized", detail)
}

// Forbidden returns a 403 problem details response.
func Forbidden(detail string) *ProblemDetails {
	return New(http.StatusForbidden, "Forbidden", detail)
}

// NotFound returns a 404 problem details response.
func NotFound(detail string) *ProblemDetails {
	return New(http.StatusNotFound, "Not Found", detail)
}

// Conflict returns a 409 problem details response.
func Conflict(detail string) *ProblemDetails {
	return New(http.StatusConflict, "Conflict", detail)
}

// UnprocessableEntity returns a 422 problem details response.
func UnprocessableEntity(detail string) *ProblemDetails {
	return New(http.StatusUnprocessableEntity, "Unprocessable Entity", detail)
}

// TooManyRequests returns a 429 problem details response.
func TooManyRequests(detail string) *ProblemDetails {
	return New(http.StatusTooManyRequests, "Too Many Requests", detail)
}

// InternalError returns a 500 problem details response.
func InternalError(detail string) *ProblemDetails {
	return New(http.StatusInternalServerError, "Internal Server Error", detail)
}

// ServiceUnavailable returns a 503 problem details response.
func ServiceUnavailable(detail string) *ProblemDetails {
	return New(http.StatusServiceUnavailable, "Service Unavailable", detail)
}

// BadGateway returns a 502 problem details response.
func BadGateway(detail string) *ProblemDetails {
	return New(http.StatusBadGateway, "Bad Gateway", detail)
}

func (e *ProblemDetails) Write(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(e.Status)
	if err := json.NewEncoder(w).Encode(e); err != nil {
		slog.Debug("problem-details write failed", "err", err)
	}
}
