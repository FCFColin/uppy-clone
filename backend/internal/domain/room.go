package domain

import "encoding/json"

// RoomInfo is a summary of a room, used for cache values and API responses.
type RoomInfo struct {
	Code        string `json:"code"`
	Phase       string `json:"phase"`
	PlayerCount int    `json:"player_count"`
	CreatedAt   int64  `json:"created_at"`
}

// RoomRegistryInfo is room ownership metadata stored in Redis (ADR-005).
type RoomRegistryInfo struct {
	Code      string `json:"code"`
	Instance  string `json:"instance"`
	Address   string `json:"address"`
	CreatedAt int64  `json:"created_at"`
}

// UnmarshalRoomRegistryInfo decodes JSON-encoded room registry metadata.
func UnmarshalRoomRegistryInfo(data []byte) (*RoomRegistryInfo, error) {
	var info RoomRegistryInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}
