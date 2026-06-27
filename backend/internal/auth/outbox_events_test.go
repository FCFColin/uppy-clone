package auth

import (
	"encoding/json"
	"testing"
)

func TestGameEndedOutboxPayload(t *testing.T) {
	t.Parallel()
	payload := map[string]interface{}{
		"session_id": "sess-123",
		"score":      100,
	}
	data, err := GameEndedOutboxPayload(payload)
	if err != nil {
		t.Fatalf("GameEndedOutboxPayload: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["event"] != "game.ended" {
		t.Errorf("event = %v, want game.ended", result["event"])
	}
}

func TestGameEndedOutboxPayload_NilPayload(t *testing.T) {
	t.Parallel()
	data, err := GameEndedOutboxPayload(nil)
	if err != nil {
		t.Fatalf("GameEndedOutboxPayload(nil): %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["data"] != nil {
		t.Errorf("data should be nil, got %v", result["data"])
	}
}

func TestGameEndedOutboxPayload_EmptyPayload(t *testing.T) {
	t.Parallel()
	data, err := GameEndedOutboxPayload(map[string]interface{}{})
	if err != nil {
		t.Fatalf("GameEndedOutboxPayload(empty): %v", err)
	}
	if len(data) == 0 {
		t.Error("result should not be empty")
	}
}
