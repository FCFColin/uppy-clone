package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/store"
)

func TestWriteDegradedJSON_Structure(t *testing.T) {
	rec := httptest.NewRecorder()

	data := map[string]string{"room": "ABC12", "players": "3"}
	WriteDegradedJSON(rec, http.StatusOK, data, "Redis unavailable")

	var resp DegradedResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Degraded != true {
		t.Errorf("degraded = %v, want true", resp.Degraded)
	}

	if resp.Message != "Redis unavailable" {
		t.Errorf("message = %q, want %q", resp.Message, "Redis unavailable")
	}

	// Data field should contain the map — decode to verify
	dataBytes, _ := json.Marshal(resp.Data)
	var result map[string]string
	if err := json.Unmarshal(dataBytes, &result); err != nil {
		t.Fatalf("failed to decode data field: %v", err)
	}
	if result["room"] != "ABC12" {
		t.Errorf("data.room = %q, want %q", result["room"], "ABC12")
	}
	if result["players"] != "3" {
		t.Errorf("data.players = %q, want %q", result["players"], "3")
	}
}

// --- Content-Type 必须是 application/json ---

func TestWriteDegradedJSON_ContentType(t *testing.T) {
	rec := httptest.NewRecorder()

	WriteDegradedJSON(rec, http.StatusOK, nil, "")

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}

// --- 状态码必须正确传递 ---

func TestWriteDegradedJSON_StatusCodes(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"200 OK", http.StatusOK},
		{"201 Created", http.StatusCreated},
		{"202 Accepted", http.StatusAccepted},
		{"206 Partial Content", http.StatusPartialContent},
		{"503 Service Unavailable", http.StatusServiceUnavailable},
		{"502 Bad Gateway", http.StatusBadGateway},
		{"500 Internal Server Error", http.StatusInternalServerError},
		{"429 Too Many Requests", http.StatusTooManyRequests},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteDegradedJSON(rec, tt.status, nil, "degraded")

			if rec.Code != tt.status {
				t.Errorf("status = %d, want %d", rec.Code, tt.status)
			}
		})
	}
}

// --- message omitempty：空字符串时 message 字段不应出现在 JSON 中 ---

func TestWriteDegradedJSON_MessageOmitempty(t *testing.T) {
	t.Run("empty message omitted", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WriteDegradedJSON(rec, http.StatusOK, nil, "")

		var raw map[string]json.RawMessage
		if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}

		if _, exists := raw["message"]; exists {
			t.Errorf("message field should be omitted when empty, but got: %s", raw["message"])
		}
	})

	t.Run("non-empty message present", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WriteDegradedJSON(rec, http.StatusOK, nil, "cache miss")

		var raw map[string]json.RawMessage
		if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}

		msgRaw, exists := raw["message"]
		if !exists {
			t.Fatal("message field should be present when non-empty")
		}

		var msg string
		if err := json.Unmarshal(msgRaw, &msg); err != nil {
			t.Fatalf("failed to unmarshal message: %v", err)
		}
		if msg != "cache miss" {
			t.Errorf("message = %q, want %q", msg, "cache miss")
		}
	})
}

// --- degraded 字段始终为 true ---

func TestWriteDegradedJSON_DegradedAlwaysTrue(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		data    interface{}
		message string
	}{
		{"200 with data", http.StatusOK, "hello", "msg"},
		{"503 with nil data", http.StatusServiceUnavailable, nil, ""},
		{"500 with empty data", http.StatusInternalServerError, "", ""},
		{"200 with slice", http.StatusOK, []int{1, 2, 3}, "partial"},
		{"200 with map", http.StatusOK, map[string]int{"a": 1}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteDegradedJSON(rec, tt.status, tt.data, tt.message)

			var resp DegradedResponse
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode: %v", err)
			}

			if !resp.Degraded {
				t.Errorf("degraded = false, want true for %s", tt.name)
			}
		})
	}
}

// --- data 字段始终存在（即使为 nil） ---

func TestWriteDegradedJSON_DataAlwaysPresent(t *testing.T) {
	t.Run("nil data", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WriteDegradedJSON(rec, http.StatusOK, nil, "")

		var raw map[string]json.RawMessage
		if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}

		if _, exists := raw["data"]; !exists {
			t.Error("data field should always be present, even when nil")
		}
	})

	t.Run("string data", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WriteDegradedJSON(rec, http.StatusOK, "partial data", "")

		var resp DegradedResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}

		var dataStr string
		dataBytes, _ := json.Marshal(resp.Data)
		if err := json.Unmarshal(dataBytes, &dataStr); err != nil {
			t.Fatalf("failed to unmarshal data: %v", err)
		}
		if dataStr != "partial data" {
			t.Errorf("data = %q, want %q", dataStr, "partial data")
		}
	})

	t.Run("slice data", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WriteDegradedJSON(rec, http.StatusOK, []string{"a", "b"}, "")

		var resp DegradedResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}

		var dataSlice []string
		dataBytes, _ := json.Marshal(resp.Data)
		if err := json.Unmarshal(dataBytes, &dataSlice); err != nil {
			t.Fatalf("failed to unmarshal data: %v", err)
		}
		if len(dataSlice) != 2 || dataSlice[0] != "a" || dataSlice[1] != "b" {
			t.Errorf("data = %v, want [a, b]", dataSlice)
		}
	})
}

// --- Require* 降级守卫函数 ---

func TestRequireDB_ReturnsTrueWhenNotNil(t *testing.T) {
	t.Parallel()

	// We can't call CreateUser/GetUserByID without a real DB, but we test
	// the nil guard (always reachable) and the non-nil path (always true).
	t.Run("nil db returns false", func(t *testing.T) {
		w := httptest.NewRecorder()
		result := RequireDB(nil, w)
		if result {
			t.Error("RequireDB(nil) = true, want false")
		}
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
		}
	})

	t.Run("non-nil db returns true", func(t *testing.T) {
		w := httptest.NewRecorder()
		// Use a non-nil *PostgresStore pointer (zero value)
		var zeroStore store.PostgresStore
		result := RequireDB(&zeroStore, w)
		if !result {
			t.Error("RequireDB(non-nil) = false, want true")
		}
	})
}

func TestRequireRedis_ReturnsTrueWhenNotNil(t *testing.T) {
	t.Parallel()

	t.Run("nil redis returns false", func(t *testing.T) {
		w := httptest.NewRecorder()
		result := RequireRedis(nil, w)
		if result {
			t.Error("RequireRedis(nil) = true, want false")
		}
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
		}
	})

	t.Run("non-nil redis returns true", func(t *testing.T) {
		w := httptest.NewRecorder()
		var zeroStore store.RedisStore
		result := RequireRedis(&zeroStore, w)
		if !result {
			t.Error("RequireRedis(non-nil) = false, want true")
		}
	})
}

func TestRequireHub_ReturnsTrueWhenNotNil(t *testing.T) {
	t.Parallel()

	t.Run("nil hub returns false", func(t *testing.T) {
		w := httptest.NewRecorder()
		result := RequireHub(nil, w)
		if result {
			t.Error("RequireHub(nil) = true, want false")
		}
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
		}

		var resp domain.ProblemDetails
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}
		if resp.Status != http.StatusServiceUnavailable {
			t.Errorf("error status = %d, want %d", resp.Status, http.StatusServiceUnavailable)
		}
	})

	t.Run("non-nil hub returns true", func(t *testing.T) {
		w := httptest.NewRecorder()
		hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0)
		result := RequireHub(hub, w)
		if !result {
			t.Error("RequireHub(non-nil) = false, want true")
		}
	})
}

func TestRequireHubDegraded_ReturnsTrueWhenNotNil(t *testing.T) {
	t.Parallel()

	t.Run("nil hub returns false with degraded JSON", func(t *testing.T) {
		w := httptest.NewRecorder()
		payload := map[string]string{"status": "degraded"}
		result := RequireHubDegraded(nil, w, http.StatusOK, payload, "Hub unavailable")
		if result {
			t.Error("RequireHubDegraded(nil) = true, want false")
		}

		var resp DegradedResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if !resp.Degraded {
			t.Error("degraded = false, want true")
		}
		if resp.Message != "Hub unavailable" {
			t.Errorf("message = %q, want %q", resp.Message, "Hub unavailable")
		}
	})

	t.Run("non-nil hub returns true", func(t *testing.T) {
		w := httptest.NewRecorder()
		hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0)
		result := RequireHubDegraded(hub, w, http.StatusOK, nil, "")
		if !result {
			t.Error("RequireHubDegraded(non-nil) = false, want true")
		}
	})
}

// --- 完整 JSON 输出验证 ---

func TestRequireDB_DegradedResponseBody(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	RequireDB(nil, w)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	var resp DegradedResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Degraded || resp.Message != "Database temporarily unavailable" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestRequireRedis_DegradedResponseBody(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	RequireRedis(nil, w)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	var resp DegradedResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Degraded || resp.Message != "Cache temporarily unavailable" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestWriteDegradedJSON_FullOutput(t *testing.T) {
	t.Run("with message", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WriteDegradedJSON(rec, http.StatusPartialContent, "partial", "cache degraded")

		body := rec.Body.String()
		if !strings.Contains(body, `"degraded":true`) {
			t.Errorf("body should contain degraded:true, got: %s", body)
		}
		if !strings.Contains(body, `"message":"cache degraded"`) {
			t.Errorf("body should contain message, got: %s", body)
		}
		if !strings.Contains(body, `"data":"partial"`) {
			t.Errorf("body should contain data, got: %s", body)
		}
	})

	t.Run("without message", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WriteDegradedJSON(rec, http.StatusOK, nil, "")

		body := rec.Body.String()
		if !strings.Contains(body, `"degraded":true`) {
			t.Errorf("body should contain degraded:true, got: %s", body)
		}
		if strings.Contains(body, `"message"`) {
			t.Errorf("body should NOT contain message field when empty, got: %s", body)
		}
		if !strings.Contains(body, `"data":null`) {
			t.Errorf("body should contain data:null for nil data, got: %s", body)
		}
	})
}
