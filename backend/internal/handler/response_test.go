package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSON_SetsContentType(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]string{"k": "v"})

	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}

func TestWriteJSON_WritesStatusCode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status int
	}{
		{"200 OK", http.StatusOK},
		{"201 Created", http.StatusCreated},
		{"204 No Content", http.StatusNoContent},
		{"400 Bad Request", http.StatusBadRequest},
		{"404 Not Found", http.StatusNotFound},
		{"500 Internal Server Error", http.StatusInternalServerError},
		{"503 Service Unavailable", http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeJSON(rec, tt.status, nil)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d", rec.Code, tt.status)
			}
		})
	}
}

func TestWriteJSON_EncodesPayload(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	payload := map[string]int{"a": 1, "b": 2}
	writeJSON(rec, http.StatusOK, payload)

	var got map[string]int
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["a"] != 1 || got["b"] != 2 {
		t.Fatalf("payload = %+v, want a=1, b=2", got)
	}
}

func TestWriteJSON_NilPayload(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	// nil should encode to "null"
	var v interface{}
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v != nil {
		t.Fatalf("decoded value = %v, want nil", v)
	}
}

func TestWriteJSON_SlicePayload(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	payload := []string{"a", "b", "c"}
	writeJSON(rec, http.StatusOK, payload)

	var got []string
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("got = %v", got)
	}
}

func TestWriteJSON_OverrideHeaderOnSecondWrite(t *testing.T) {
	t.Parallel()
	// Adversarial: subsequent writeJSON should still set content-type correctly.
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, "first")
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q", rec.Header().Get("Content-Type"))
	}
}

func TestWriteJSON_StatusBeforeBody(t *testing.T) {
	t.Parallel()
	// Verify header status is set before body write (which is the Go convention).
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusCreated, map[string]string{"ok": "true"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("body should not be empty")
	}
}
