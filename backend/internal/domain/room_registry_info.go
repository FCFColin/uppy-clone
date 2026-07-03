package domain

// RoomRegistryInfo is room ownership metadata stored in Redis (ADR-005).
type RoomRegistryInfo struct {
	Code      string `json:"code"`
	Instance  string `json:"instance"`
	Address   string `json:"address"`
	CreatedAt int64  `json:"created_at"`
}
