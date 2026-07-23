package config

import (
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
		JWTPrivateKey: "DEV_ONLY_secret_012345678901234567890",
		DatabaseURL:   "postgres://localhost/test",
		EncryptionKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		EnableHSTS:    true,
		Environment:   "production",
	}
	if err := e.Validate(); err == nil {
		t.Fatal("expected weak JWT rejection in production mode")
	}
}

func TestEnvValidate_DevMode(t *testing.T) {
	e := Load()
	e.EnableHSTS = false
	e.JWTPrivateKey = "dev-only-test-secret-32bytes!!"
	e.DatabaseURL = "postgres://localhost/test"
	e.EncryptionKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := e.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
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

func TestEnvValidate_RejectsDevOnlyMarker(t *testing.T) {
	e := validProdEnv()
	e.JWTPrivateKey = "my-DEV_ONLY-secret-key-at-least-32-bytes-long!!"
	if err := e.Validate(); err == nil {
		t.Fatal("expected weak JWT rejection for DEV_ONLY marker")
	}
}

func TestIsWeakJWTSecret_RejectsChangeInProduction(t *testing.T) {
	e := validProdEnv()
	e.JWTPrivateKey = "strong-change-in-production-secret-32bytes!"
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

func validProdEnv() *Env {
	return &Env{
		JWTPrivateKey:     "strong-production-jwt-secret-32bytes!",
		JWTPublicKey:      "strong-production-jwt-public-key-32bytes!",
		DatabaseURL:       "postgres://localhost/test",
		EncryptionKey:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		AuditSecret:       "audit-secret-independent-from-jwt-32bytes!",
		EnableHSTS:        true,
		Environment:       "production",
		TrustedProxyCIDRs: "127.0.0.1/32",
	}
}

func TestEnv_AuditSecretOrJWT(t *testing.T) {
	e := &Env{AuditSecret: "audit", JWTPrivateKey: "jwt"}
	if got := e.AuditSecretOrJWT(); got != "audit" {
		t.Errorf("got %q", got)
	}
	e.AuditSecret = ""
	if got := e.AuditSecretOrJWT(); got != "jwt" {
		t.Errorf("fallback got %q", got)
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
	// Bare integer is interpreted as seconds (legacy operator format, v2-R-38).
	t.Setenv("TEST_DURATION", "90")
	if got := GetEnvDuration("TEST_DURATION", def); got != 90*time.Second {
		t.Errorf("bare-int got %v", got)
	}
}

func TestParseRedisURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		raw      string
		wantAddr string
		wantPass string
		wantDB   int
		wantErr  bool
	}{
		{"", "", "", 0, true},
		{"localhost:6379", "localhost:6379", "", 0, false},
		{"redis:6379", "redis:6379", "", 0, false},
		{"redis://:secret@redis:6379", "redis:6379", "secret", 0, false},
		{"redis://redis:6379/0", "redis:6379", "", 0, false},
		{"redis://redis:6379/2", "redis:6379", "", 2, false},
		{"rediss://:pass@secure:6380/1", "secure:6380", "pass", 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			t.Parallel()
			got, err := ParseRedisURL(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRedisURL: %v", err)
			}
			if got.Addr != tt.wantAddr || got.Password != tt.wantPass || got.DB != tt.wantDB { // pragma: allowlist secret
				t.Fatalf("got %+v want addr=%q pass=%q db=%d", got, tt.wantAddr, tt.wantPass, tt.wantDB)
			}
		})
	}
}

func TestParseRedisURL_MissingHost(t *testing.T) {
	_, err := ParseRedisURL("redis://")
	if err == nil {
		t.Fatal("expected error for redis URL without host")
	}
}

func TestParseRedisURL_InvalidDB(t *testing.T) {
	_, err := ParseRedisURL("redis://localhost:6379/abc")
	if err == nil {
		t.Fatal("expected error for invalid DB number")
	}
}
