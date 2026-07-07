// Package domain holds core game and user model types shared across layers.
package domain

// AppConfig stores admin configuration as JSON.
// Note: this is a stored config type (not pure domain).
type AppConfig struct {
	ID           string `json:"id"`
	Config       string `json:"config"`
	UpdatedAt    int64  `json:"updated_at"`
	EmailEnabled bool   `json:"email_enabled"`
	EmailFrom    string `json:"email_from"`
}
