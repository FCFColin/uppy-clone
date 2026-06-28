// Package domain holds core game and user model types shared across layers.
package domain

// AppConfig stores admin configuration as JSON.
type AppConfig struct {
	ID            string `json:"id"`
	Config        string `json:"config"`
	UpdatedAt     int64  `json:"updated_at"`
	EmailEnabled  bool   `json:"email_enabled"`
	ResendAPIKey  string `json:"resend_api_key"`
	EmailFrom     string `json:"email_from"`
	AdminPassword string `json:"admin_password"`
}
