package main

import (
	"strings"
	"testing"
)

func TestRunSeed_MissingURL(t *testing.T) {
	_, err := runSeed("")
	if err == nil {
		t.Fatal("expected error for empty DATABASE_URL")
	}
}

func TestRunSeed_RejectsProductionURLPatterns(t *testing.T) {
	reject := []struct {
		name string
		url  string
	}{
		{"sslmode require", "postgres://u:p@prod.example.com:5432/app?sslmode=require"},
		{"sslmode verify-full", "postgres://u:p@db.internal:5432/app?sslmode=verify-full"},
		{"sslmode verify-ca", "postgres://u:p@host/db?sslmode=verify-ca"},
		{"sslmode prefer", "postgres://u:p@host/db?sslmode=prefer"},
		{"missing sslmode", "postgres://u:p@prod.rds.amazonaws.com:5432/app"},
		{"disable only in password", "postgres://u:sslmode%3Ddisable@host/db?sslmode=require"},
		{"disable substring in host", "postgres://u:p@sslmode-disable.example/db?sslmode=require"},
		{"uppercase require", "postgres://u:p@host/db?SSLMODE=REQUIRE"},
		{"heroku production style", "postgres://u:p@ec2-1-2-3-4.compute-1.amazonaws.com:5432/d123?sslmode=require"},
		{"cockroach cloud", "postgresql://user:pass@gcp-us-east1.cockroachlabs.cloud:26257/defaultdb?sslmode=verify-full"},
	}

	for _, tc := range reject {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runSeed(tc.url)
			if err == nil {
				t.Fatalf("expected rejection for %q", tc.url)
			}
			if !strings.Contains(err.Error(), "SEED ABORTED") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestRunSeed_AcceptsDevURLPattern(t *testing.T) {
	_, err := runSeed("postgres://u:p@127.0.0.1:1/dev?sslmode=disable")
	if err == nil {
		t.Fatal("expected connect error after guard passes")
	}
	if strings.Contains(err.Error(), "SEED ABORTED") {
		t.Fatalf("dev URL should pass guard, got %v", err)
	}
}

// v2-R-95: The bypass ?sslmode=disable&sslmode=require must now be REJECTED.
// Previously the substring guard accepted it (libpq takes the last value =
// require, but substring check found "disable"). The URL-parsed guard now
// rejects duplicate sslmode keys and checks the final value.
func TestRunSeed_GuardRejectsSSLModeBypass(t *testing.T) {
	_, err := runSeed("postgres://u:p@host/db?sslmode=disable&sslmode=require")
	if err == nil {
		t.Fatal("expected rejection for sslmode bypass attempt")
	}
	if !strings.Contains(err.Error(), "SEED ABORTED") {
		t.Fatalf("error = %v, want SEED ABORTED", err)
	}
}

// v2-R-95: Even a single sslmode=require must be rejected (not just bypass).
func TestRunSeed_GuardRejectsRequire(t *testing.T) {
	_, err := runSeed("postgres://u:p@host/db?sslmode=require")
	if err == nil {
		t.Fatal("expected rejection for sslmode=require")
	}
	if !strings.Contains(err.Error(), "SEED ABORTED") {
		t.Fatalf("error = %v, want SEED ABORTED", err)
	}
}

// v2-R-95: Reverse bypass ?sslmode=require&sslmode=disable must also be
// rejected (duplicate sslmode keys are ambiguous even if the last is disable).
func TestRunSeed_GuardRejectsReverseBypass(t *testing.T) {
	_, err := runSeed("postgres://u:p@host/db?sslmode=require&sslmode=disable")
	if err == nil {
		t.Fatal("expected rejection for duplicate sslmode keys")
	}
	if !strings.Contains(err.Error(), "SEED ABORTED") {
		t.Fatalf("error = %v, want SEED ABORTED", err)
	}
}
