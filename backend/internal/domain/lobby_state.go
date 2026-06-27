package domain

// LobbyState stores serialized game state for a lobby room.
type LobbyState struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	State     string `json:"state"`
	UpdatedAt int64  `json:"updated_at"`
	CreatedAt int64  `json:"created_at"`
}
