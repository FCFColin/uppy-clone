package domain

import (
	"encoding/json"
	"testing"
)

func TestRoomInfo_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	original := RoomInfo{
		Code:        "ABC12",
		Phase:       "waiting",
		PlayerCount: 4,
		CreatedAt:   1700000000,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var restored RoomInfo
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if restored != original {
		t.Fatalf("round-trip mismatch: got %+v, want %+v", restored, original)
	}
}

func TestRoomInfo_JSONFieldNames(t *testing.T) {
	t.Parallel()
	data, _ := json.Marshal(RoomInfo{Code: "X1"})
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	expectedKeys := []string{"code", "phase", "player_count", "created_at"}
	for _, key := range expectedKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q in RoomInfo output", key)
		}
	}
}

func TestRoomRegistryInfo_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	original := RoomRegistryInfo{
		Code:      "XYZ99",
		Instance:  "instance-1",
		Address:   "10.0.0.1:8080",
		CreatedAt: 1700000000,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var restored RoomRegistryInfo
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if restored != original {
		t.Fatalf("round-trip mismatch: got %+v, want %+v", restored, original)
	}
}

func TestLobbyState_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	original := LobbyState{
		ID:        "id-1",
		Code:      "ABC12",
		State:     `{"phase":"waiting"}`,
		UpdatedAt: 1700000001,
		CreatedAt: 1700000000,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var restored LobbyState
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if restored != original {
		t.Fatalf("round-trip mismatch: got %+v, want %+v", restored, original)
	}
}

func TestLobbyListResult_EmbeddedFields(t *testing.T) {
	t.Parallel()
	result := LobbyListResult{
		Lobbies:    []LobbyState{{Code: "A"}},
		Total:      1,
		HasMore:    true,
		NextCursor: "2025-01-01|A",
	}
	if len(result.Lobbies) != 1 {
		t.Fatalf("Lobbies = %d, want 1", len(result.Lobbies))
	}
	if !result.HasMore {
		t.Fatal("HasMore should be true")
	}
}

func TestLeaderboardEntry_JSONTags(t *testing.T) {
	t.Parallel()
	entry := LeaderboardEntry{
		Rank:      1,
		Score:     1000,
		LobbyCode: "ABC12",
		EndedAt:   1700000000,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	expectedKeys := []string{"rank", "score", "lobbyCode", "endedAt"}
	for _, key := range expectedKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q in LeaderboardEntry output", key)
		}
	}
}
