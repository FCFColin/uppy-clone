package config

import (
	"os"
	"testing"
	"time"
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

func TestEnvValidate_RequiresTrustedProxyCIDRsInProd(t *testing.T) {
	e := validProdEnv()
	e.TrustedProxyCIDRs = ""
	if err := e.Validate(); err == nil {
		t.Fatal("expected missing TRUSTED_PROXY_CIDRS error in production mode")
	}
}

func TestEnvValidate_RejectsInvalidTrustedProxyCIDRsInProd(t *testing.T) {
	e := validProdEnv()
	e.TrustedProxyCIDRs = "not-a-cidr"
	if err := e.Validate(); err == nil {
		t.Fatal("expected invalid TRUSTED_PROXY_CIDRS error in production mode")
	}
}

func TestEnvValidate_RejectsEmptyTrustedProxyCIDRListInProd(t *testing.T) {
	e := validProdEnv()
	e.TrustedProxyCIDRs = " , "
	if err := e.Validate(); err == nil {
		t.Fatal("expected no valid CIDR entries error in production mode")
	}
}

func TestEnvValidate_AcceptsValidTrustedProxyCIDRsInProd(t *testing.T) {
	e := validProdEnv()
	e.TrustedProxyCIDRs = "10.0.0.0/8,127.0.0.1"
	if err := e.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func validProdEnv() *Env {
	return &Env{
		JWTSecret:         "strong-production-jwt-secret-32bytes!",
		AdminJWTSecret:    "strong-production-admin-jwt-secret-32b!",
		DatabaseURL:       "postgres://localhost/test",
		EncryptionKey:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		EnableHSTS:        true,
		TrustedProxyCIDRs: "127.0.0.1/32",
	}
}

func TestEnvValidate_AdminJWTTooShort(t *testing.T) {
	e := validProdEnv()
	e.AdminJWTSecret = "too-short"
	if err := e.Validate(); err == nil {
		t.Fatal("expected ADMIN_JWT_SECRET length error")
	}
}

func TestEnv_AdminJWTSecretOrUser(t *testing.T) {
	e := &Env{AdminJWTSecret: "admin-secret", JWTSecret: "jwt-secret"}
	if got := e.AdminJWTSecretOrUser(); got != "admin-secret" {
		t.Errorf("got %q", got)
	}
	e.AdminJWTSecret = ""
	if got := e.AdminJWTSecretOrUser(); got != "jwt-secret" {
		t.Errorf("fallback got %q", got)
	}
}

func TestEnv_AuditSecretOrJWT(t *testing.T) {
	e := &Env{AuditSecret: "audit", JWTSecret: "jwt"}
	if got := e.AuditSecretOrJWT(); got != "audit" {
		t.Errorf("got %q", got)
	}
	e.AuditSecret = ""
	if got := e.AuditSecretOrJWT(); got != "jwt" {
		t.Errorf("fallback got %q", got)
	}
}

func TestGetEnvInt(t *testing.T) {
	t.Setenv("TEST_GET_ENV_INT", "42")
	if got := GetEnvInt("TEST_GET_ENV_INT", 1); got != 42 {
		t.Errorf("got %d", got)
	}
	t.Setenv("TEST_GET_ENV_INT", "bad")
	if got := GetEnvInt("TEST_GET_ENV_INT", 7); got != 7 {
		t.Errorf("invalid got %d", got)
	}
	os.Unsetenv("TEST_GET_ENV_INT")
	if got := GetEnvInt("TEST_GET_ENV_INT", 9); got != 9 {
		t.Errorf("default got %d", got)
	}
}

func TestGetEnv(t *testing.T) {
	t.Setenv("TEST_GET_ENV", "from-env")
	if got := GetEnv("TEST_GET_ENV", "default"); got != "from-env" {
		t.Errorf("got %q", got)
	}
	os.Unsetenv("TEST_GET_ENV")
	if got := GetEnv("TEST_GET_ENV", "default"); got != "default" {
		t.Errorf("default got %q", got)
	}
}

func TestGetEnvIntPositive(t *testing.T) {
	t.Setenv("TEST_POS_INT", "0")
	if got := GetEnvIntPositive("TEST_POS_INT", 5); got != 5 {
		t.Errorf("zero got %d", got)
	}
	t.Setenv("TEST_POS_INT", "-1")
	if got := GetEnvIntPositive("TEST_POS_INT", 5); got != 5 {
		t.Errorf("negative got %d", got)
	}
	t.Setenv("TEST_POS_INT", "10")
	if got := GetEnvIntPositive("TEST_POS_INT", 5); got != 10 {
		t.Errorf("positive got %d", got)
	}
	t.Setenv("TEST_POS_INT", "not-int")
	if got := GetEnvIntPositive("TEST_POS_INT", 5); got != 5 {
		t.Errorf("invalid got %d", got)
	}
	os.Unsetenv("TEST_POS_INT")
	if got := GetEnvIntPositive("TEST_POS_INT", 9); got != 9 {
		t.Errorf("unset got %d", got)
	}
}

func TestEnvValidate_RejectsDevOnlyMarker(t *testing.T) {
	e := validProdEnv()
	e.JWTSecret = "my-DEV_ONLY-secret-key-at-least-32-bytes-long!!"
	if err := e.Validate(); err == nil {
		t.Fatal("expected weak JWT rejection for DEV_ONLY marker")
	}
}

func TestIsWeakJWTSecret_RejectsChangeInProduction(t *testing.T) {
	e := validProdEnv()
	e.JWTSecret = "strong-change-in-production-secret-32bytes!"
	if err := e.Validate(); err == nil {
		t.Fatal("expected weak JWT rejection for change-in-production marker")
	}
}

func TestValidateTrustedProxyCIDRs_Empty(t *testing.T) {
	if err := validateTrustedProxyCIDRs(""); err == nil {
		t.Fatal("expected error for empty TRUSTED_PROXY_CIDRS")
	}
}

func TestValidateTrustedProxyCIDRs_SkipsEmptyParts(t *testing.T) {
	if err := validateTrustedProxyCIDRs("10.0.0.0/8,,127.0.0.1/32"); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestGetEnvDuration(t *testing.T) {
	def := 30 * time.Second
	t.Setenv("TEST_DURATION", "2m")
	if got := GetEnvDuration("TEST_DURATION", def); got != 2*time.Minute {
		t.Errorf("got %v", got)
	}
	t.Setenv("TEST_DURATION", "invalid")
	if got := GetEnvDuration("TEST_DURATION", def); got != def {
		t.Errorf("invalid got %v", got)
	}
	t.Setenv("TEST_DURATION", "-1s")
	if got := GetEnvDuration("TEST_DURATION", def); got != def {
		t.Errorf("non-positive got %v", got)
	}
}
