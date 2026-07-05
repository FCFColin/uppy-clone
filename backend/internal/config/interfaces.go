package config

// AuthConfig defines auth-related configuration methods.
type AuthConfig interface {
	GetJWTPrivateKey() string
	GetJWTPublicKey() string
	GetEncryptionKey() string
	GetAuditSecret() string
}

// DBConfig defines database configuration methods.
type DBConfig interface {
	GetDatabaseURL() string
}

// RedisConfig defines Redis configuration methods.
type RedisConfig interface {
	GetRedisURL() string
}

// ServerConfig defines server configuration methods.
type ServerConfig interface {
	GetPort() string
	GetEnableHSTS() bool
}

// GameConfig defines game configuration methods.
type GameConfig interface {
	GetMaxPlayersPerRoom() int
}

var (
	_ AuthConfig   = (*Env)(nil)
	_ DBConfig     = (*Env)(nil)
	_ RedisConfig  = (*Env)(nil)
	_ ServerConfig = (*Env)(nil)
	_ GameConfig   = (*Env)(nil)
)
