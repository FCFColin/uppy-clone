package handler

// Config holds application configuration passed to handlers.
type Config struct {
	ResendAPIKey   string
	EmailFrom      string
	AdminPassword  string
	JWTPrivateKey  string
	JWTPublicKey   string
	DatabaseURL    string
	RedisURL       string
	RedisEphemeralURL string
	RedisPubSubURL string
	Port           string
	FrontendDir    string
}
