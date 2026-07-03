package domain

// LeaderboardEntry is a single row on the public leaderboard.
type LeaderboardEntry struct {
	Rank      int    `json:"rank"`
	Score     int    `json:"score"`
	LobbyCode string `json:"lobbyCode"`
	EndedAt   int64  `json:"endedAt"`
}
