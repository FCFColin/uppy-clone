// Package apierror provides RFC 7807 problem details for HTTP API errors.
package apierror

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ProblemDetails implements RFC 7807 Problem Details for HTTP APIs.
//
// Enterprise rationale: Unified error format enables frontend to use a single
// handleApiError() function for all API errors. RFC 7807 is the IETF standard
// adopted by Google, Microsoft, Stripe. The "type" field allows machine-readable
// error classification. Trade-off: "type" URI namespace requires maintenance;
// we use https://httpstatuses.com/{status} as default, extensible later.
type ProblemDetails struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

// New builds an RFC 7807 problem response for the given HTTP status.
func New(status int, title, detail string) *ProblemDetails {
	return &ProblemDetails{
		Type:   fmt.Sprintf("https://httpstatuses.com/%d", status),
		Title:  title,
		Status: status,
		Detail: detail,
	}
}

// BadRequest returns a 400 problem response.
func BadRequest(detail string) *ProblemDetails {
	return New(http.StatusBadRequest, "Bad Request", detail)
}

// Unauthorized returns a 401 problem response.
func Unauthorized(detail string) *ProblemDetails {
	return New(http.StatusUnauthorized, "Unauthorized", detail)
}

// Forbidden returns a 403 problem response.
func Forbidden(detail string) *ProblemDetails {
	return New(http.StatusForbidden, "Forbidden", detail)
}

// NotFound returns a 404 problem response.
func NotFound(detail string) *ProblemDetails {
	return New(http.StatusNotFound, "Not Found", detail)
}

// Conflict returns a 409 problem response.
func Conflict(detail string) *ProblemDetails {
	return New(http.StatusConflict, "Conflict", detail)
}

// UnprocessableEntity returns a 422 problem response.
func UnprocessableEntity(detail string) *ProblemDetails {
	return New(http.StatusUnprocessableEntity, "Unprocessable Entity", detail)
}

// TooManyRequests returns a 429 problem response.
func TooManyRequests(detail string) *ProblemDetails {
	return New(http.StatusTooManyRequests, "Too Many Requests", detail)
}

// InternalError returns a 500 problem response.
func InternalError(detail string) *ProblemDetails {
	return New(http.StatusInternalServerError, "Internal Server Error", detail)
}

// Write writes the problem details as a JSON response with the correct
// Content-Type header per RFC 7807.
func (e *ProblemDetails) Write(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(e.Status)
	_ = json.NewEncoder(w).Encode(e)
}
