package main

import (
	"strings"
	"testing"
)

func TestRunMigrate_ConnectionFailure(t *testing.T) {
	_, err := runMigrate("postgres://invalid:invalid@127.0.0.1:1/nodb?sslmode=disable")
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "connect") {
		t.Fatalf("error = %v", err)
	}
}

func TestMigrationStatusAfterLoad_BcryptShortCircuit(t *testing.T) {
	hashed := "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
	status, done := migrationStatusAfterLoad(hashed)
	if !done {
		t.Fatal("expected bcrypt password to short-circuit migration")
	}
	if status != "already bcrypt" {
		t.Fatalf("status = %q", status)
	}

	status, done = migrationStatusAfterLoad("plain-secret")
	if done {
		t.Fatalf("plaintext should not short-circuit, status=%q", status)
	}
}
