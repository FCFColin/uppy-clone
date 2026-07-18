package domain

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProblemDetails_Constructors(t *testing.T) {
	tests := []struct {
		name   string
		pd     *ProblemDetails
		status int
		title  string
	}{
		{"BadRequest", BadRequest("bad"), http.StatusBadRequest, "Bad Request"},
		{"Unauthorized", Unauthorized("no auth"), http.StatusUnauthorized, "Unauthorized"},
		{"Forbidden", Forbidden("denied"), http.StatusForbidden, "Forbidden"},
		{"NotFound", NotFound("missing"), http.StatusNotFound, "Not Found"},
		{"Conflict", Conflict("dup"), http.StatusConflict, "Conflict"},
		{"UnprocessableEntity", UnprocessableEntity("invalid"), http.StatusUnprocessableEntity, "Unprocessable Entity"},
		{"TooManyRequests", TooManyRequests("slow down"), http.StatusTooManyRequests, "Too Many Requests"},
		{"InternalError", InternalError("boom"), http.StatusInternalServerError, "Internal Server Error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.pd.Status != tt.status || tt.pd.Title != tt.title {
				t.Fatalf("status/title mismatch: %+v", tt.pd)
			}
			wantType := fmt.Sprintf("https://httpstatuses.com/%d", tt.status)
			if tt.pd.Type != wantType {
				t.Errorf("type = %q, want %q", tt.pd.Type, wantType)
			}
		})
	}
}

func TestProblemDetails_Write(t *testing.T) {
	// Adversarial: malformed client must receive RFC7807 JSON, not HTML error page.
	pd := New(http.StatusForbidden, "Forbidden", "X-User-Role header spoofing denied")
	rec := httptest.NewRecorder()
	pd.Write(rec)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var got ProblemDetails
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Detail != pd.Detail || got.Status != pd.Status {
		t.Errorf("body mismatch: %+v", got)
	}
}
