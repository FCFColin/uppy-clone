// Package apierror provides RFC 7807 problem details for HTTP API errors.
package apierror

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type ProblemDetails struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

func New(status int, title, detail string) *ProblemDetails {
	return &ProblemDetails{
		Type:   fmt.Sprintf("https://httpstatuses.com/%d", status),
		Title:  title,
		Status: status,
		Detail: detail,
	}
}

func BadRequest(detail string) *ProblemDetails {
	return New(http.StatusBadRequest, "Bad Request", detail)
}

func Unauthorized(detail string) *ProblemDetails {
	return New(http.StatusUnauthorized, "Unauthorized", detail)
}

func Forbidden(detail string) *ProblemDetails {
	return New(http.StatusForbidden, "Forbidden", detail)
}

func NotFound(detail string) *ProblemDetails {
	return New(http.StatusNotFound, "Not Found", detail)
}

func Conflict(detail string) *ProblemDetails {
	return New(http.StatusConflict, "Conflict", detail)
}

func UnprocessableEntity(detail string) *ProblemDetails {
	return New(http.StatusUnprocessableEntity, "Unprocessable Entity", detail)
}

func TooManyRequests(detail string) *ProblemDetails {
	return New(http.StatusTooManyRequests, "Too Many Requests", detail)
}

func InternalError(detail string) *ProblemDetails {
	return New(http.StatusInternalServerError, "Internal Server Error", detail)
}

func (e *ProblemDetails) Write(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(e.Status)
	_ = json.NewEncoder(w).Encode(e)
}
