package domain

// LobbyListResult contains paginated lobby results with metadata.
type LobbyListResult struct {
	Lobbies    []LobbyState
	Total      int
	HasMore    bool
	NextCursor string // format: "updated_at|code"
}
