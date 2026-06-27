package main

import (
	"strings"
	"testing"
)

func TestRunSeed_MissingURL(t *testing.T) {
	if err := runSeed(""); err == nil {
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
			err := runSeed(tc.url)
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
	err := runSeed("postgres://u:p@127.0.0.1:1/dev?sslmode=disable")
	if err == nil {
		t.Fatal("expected connect error after guard passes")
	}
	if strings.Contains(err.Error(), "SEED ABORTED") {
		t.Fatalf("dev URL should pass guard, got %v", err)
	}
}

func TestRunSeed_WeakGuardSubstringBypass(t *testing.T) {
	err := runSeed("postgres://u:p@host/db?sslmode=disable&sslmode=require")
	if err != nil && strings.Contains(err.Error(), "SEED ABORTED") {
		t.Fatal("substring guard accepts sslmode=disable even when require is also present")
	}
}
