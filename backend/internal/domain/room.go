package domain

import (
	"encoding/json"
	"fmt"
)

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

// RoomCode is a value object representing a 5-character room code.
// 字符集为 [A-Z2-9]（去除易混淆的 0/1/I/O）。
type RoomCode string

// NewRoomCode creates a RoomCode, returning an error if invalid.
func NewRoomCode(code string) (RoomCode, error) {
	if len(code) != 5 {
		return "", fmt.Errorf("room code must be 5 characters, got %d", len(code))
	}
	for _, c := range code {
		if (c < 'A' || c > 'Z') && (c < '2' || c > '9') {
			return "", fmt.Errorf("room code contains invalid character: %c", c)
		}
	}
	return RoomCode(code), nil
}

// String returns the string representation.
func (r RoomCode) String() string {
	return string(r)
}

// LobbyState stores serialized game state for a lobby room.
type LobbyState struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	State     string `json:"state"`
	UpdatedAt int64  `json:"updated_at"`
	CreatedAt int64  `json:"created_at"`
}

// LobbyListResult contains paginated lobby results with metadata.
type LobbyListResult struct {
	Lobbies    []LobbyState
	Total      int
	HasMore    bool
	NextCursor string // format: "updated_at|code"
}

// LeaderboardEntry is a single row on the public leaderboard.
type LeaderboardEntry struct {
	Rank    int    `json:"rank"`
	Score   int    `json:"score"`
	Name    string `json:"name"`
	EndedAt int64  `json:"endedAt"`
}
