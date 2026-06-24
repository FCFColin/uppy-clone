package handler

// Config holds application configuration passed to handlers.
type Config struct {
	ResendAPIKey  string
	EmailFrom     string
	AdminPassword string
	JWTSecret     string
	DatabaseURL   string
	RedisURL      string
	Port          string
	FrontendDir   string
}
