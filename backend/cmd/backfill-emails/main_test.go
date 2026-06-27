package main

import (
	"os"
	"strings"
	"testing"
)

func TestRun_MissingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	err := run()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL missing")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("error = %v", err)
	}
}

func TestRun_MissingEncryptionKey(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@127.0.0.1:1/db?sslmode=disable")
	_ = os.Unsetenv("ENCRYPTION_KEY")
	err := run()
	if err == nil {
		t.Fatal("expected error when ENCRYPTION_KEY missing")
	}
}
