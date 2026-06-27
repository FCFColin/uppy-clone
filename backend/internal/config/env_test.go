package config

import (
	"os"
	"testing"
)

func TestEnvValidate_RequiresJWT(t *testing.T) {
	e := &Env{EnableHSTS: true}
	if err := e.Validate(); err == nil {
		t.Fatal("expected missing JWT_SECRET error")
	}
}

func TestEnvValidate_RejectsWeakJWTInProd(t *testing.T) {
	e := &Env{
		JWTSecret:     "DEV_ONLY_secret_012345678901234567890",
		DatabaseURL:   "postgres://localhost/test",
		EncryptionKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		EnableHSTS:    true,
	}
	if err := e.Validate(); err == nil {
		t.Fatal("expected weak JWT rejection in production mode")
	}
}

func TestEnvValidate_DevMode(t *testing.T) {
	e := Load()
	e.EnableHSTS = false
	e.JWTSecret = "dev-only-test-secret-32bytes!!"
	e.DatabaseURL = "postgres://localhost/test"
	e.EncryptionKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := e.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	_ = os.Getenv("JWT_SECRET")
}
