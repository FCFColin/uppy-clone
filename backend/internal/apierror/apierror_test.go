package apierror

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProblemDetails_Write(t *testing.T) {
	t.Parallel()

	for _, tt := range problemDetailsTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			tt.pd.Write(w)

			assertProblemDetailsResponse(t, w, tt)
		})
	}
}

// problemDetailsTestCase represents a single ProblemDetails write scenario.
type problemDetailsTestCase struct {
	name       string
	pd         *ProblemDetails
	wantStatus int
	wantType   string
	wantTitle  string
	wantDetail string
}

// problemDetailsTestCases returns the table of ProblemDetails write scenarios.
func problemDetailsTestCases() []problemDetailsTestCase {
	return append(clientErrorCases(), serverErrorCases()...)
}

// clientErrorCases returns test cases for 4xx client errors.
func clientErrorCases() []problemDetailsTestCase {
	return []problemDetailsTestCase{
		{
			name:       "bad request",
			pd:         BadRequest("missing field"),
			wantStatus: http.StatusBadRequest,
			wantType:   "https://httpstatuses.com/400",
			wantTitle:  "Bad Request",
			wantDetail: "missing field",
		},
		{
			name:       "unauthorized",
			pd:         Unauthorized("invalid token"),
			wantStatus: http.StatusUnauthorized,
			wantType:   "https://httpstatuses.com/401",
			wantTitle:  "Unauthorized",
			wantDetail: "invalid token",
		},
		{
			name:       "forbidden",
			pd:         Forbidden("no access"),
			wantStatus: http.StatusForbidden,
			wantType:   "https://httpstatuses.com/403",
			wantTitle:  "Forbidden",
			wantDetail: "no access",
		},
		{
			name:       "not found",
			pd:         NotFound("resource gone"),
			wantStatus: http.StatusNotFound,
			wantType:   "https://httpstatuses.com/404",
			wantTitle:  "Not Found",
			wantDetail: "resource gone",
		},
	}
}

// serverErrorCases returns test cases for 4xx/5xx server errors.
func serverErrorCases() []problemDetailsTestCase {
	return []problemDetailsTestCase{
		{
			name:       "conflict",
			pd:         Conflict("duplicate"),
			wantStatus: http.StatusConflict,
			wantType:   "https://httpstatuses.com/409",
			wantTitle:  "Conflict",
			wantDetail: "duplicate",
		},
		{
			name:       "too many requests",
			pd:         TooManyRequests("slow down"),
			wantStatus: http.StatusTooManyRequests,
			wantType:   "https://httpstatuses.com/429",
			wantTitle:  "Too Many Requests",
			wantDetail: "slow down",
		},
		{
			name:       "internal error",
			pd:         InternalError("db down"),
			wantStatus: http.StatusInternalServerError,
			wantType:   "https://httpstatuses.com/500",
			wantTitle:  "Internal Server Error",
			wantDetail: "db down",
		},
		{
			name:       "unprocessable entity",
			pd:         UnprocessableEntity("bad data"),
			wantStatus: http.StatusUnprocessableEntity,
			wantType:   "https://httpstatuses.com/422",
			wantTitle:  "Unprocessable Entity",
			wantDetail: "bad data",
		},
	}
}

// assertProblemDetailsResponse verifies the recorder output matches the expected test case.
func assertProblemDetailsResponse(t *testing.T, w *httptest.ResponseRecorder, tt problemDetailsTestCase) {
	t.Helper()

	if w.Code != tt.wantStatus {
		t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/problem+json")
	}

	var got ProblemDetails
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if got.Type != tt.wantType {
		t.Errorf("type = %q, want %q", got.Type, tt.wantType)
	}
	if got.Title != tt.wantTitle {
		t.Errorf("title = %q, want %q", got.Title, tt.wantTitle)
	}
	if got.Detail != tt.wantDetail {
		t.Errorf("detail = %q, want %q", got.Detail, tt.wantDetail)
	}
	if got.Status != tt.wantStatus {
		t.Errorf("status field = %d, want %d", got.Status, tt.wantStatus)
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	pd := New(418, "I'm a teapot", "short and stout")
	if pd.Status != 418 {
		t.Errorf("Status = %d, want 418", pd.Status)
	}
	if pd.Title != "I'm a teapot" {
		t.Errorf("Title = %q, want %q", pd.Title, "I'm a teapot")
	}
	if pd.Detail != "short and stout" {
		t.Errorf("Detail = %q, want %q", pd.Detail, "short and stout")
	}
	if pd.Type != "https://httpstatuses.com/418" {
		t.Errorf("Type = %q, want %q", pd.Type, "https://httpstatuses.com/418")
	}
}
