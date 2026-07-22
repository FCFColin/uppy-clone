// Package domain holds core game and user model types shared across layers.
package domain

// User represents a registered user.
type User struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Nickname  string `json:"nickname"`
	Palette   int    `json:"palette"`
	CreatedAt int64  `json:"created_at"`
	LastLogin *int64 `json:"last_login"`
}

// GameSession represents a game session record.
// store-024: LobbyCode is a plain string for historical compatibility. A typed
// LobbyCode alias would improve safety but requires changes across store/handler/
// protocol layers — deferred as low-priority refactoring.
type GameSession struct {
	ID         string  `json:"id"`
	LobbyCode  string  `json:"lobby_code"`
	CreatedBy  *string `json:"created_by"`
	Status     string  `json:"status"`
	StartedAt  *int64  `json:"started_at"`
	EndedAt    *int64  `json:"ended_at"`
	FinalScore int     `json:"final_score"`
}

// GameResult represents a single player result in a game session.
type GameResult struct {
	ID                string `json:"id"`
	SessionID         string `json:"session_id"`
	UserID            string `json:"user_id"`
	ScoreContribution int    `json:"score_contribution"`
	TapsCount         int    `json:"taps_count"`
	CreatedAt         int64  `json:"created_at"`
}

// GameResultPlayer is a lightweight player result for direct DB writes.
type GameResultPlayer struct {
	UserID            string `json:"user_id"`
	Nickname          string `json:"nickname"`
	ScoreContribution int    `json:"score_contribution"`
	TapsCount         int    `json:"taps_count"`
}

// AppConfig stores admin configuration as JSON.
// Note: this is a stored config type (not pure domain).
type AppConfig struct {
	ID           string `json:"id"`
	Config       string `json:"config"`
	UpdatedAt    int64  `json:"updated_at"`
	EmailEnabled bool   `json:"email_enabled"`
	EmailFrom    string `json:"email_from"`
}
