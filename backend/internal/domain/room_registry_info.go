package domain

import "encoding/json"

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
