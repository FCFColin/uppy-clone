package domain

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
	Rank      int    `json:"rank"`
	Score     int    `json:"score"`
	LobbyCode string `json:"lobbyCode"`
	EndedAt   int64  `json:"endedAt"`
}
