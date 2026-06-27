package store

import (
	"testing"

	"github.com/uppy-clone/backend/internal/config"
)

func TestPostgresStore_NewRequiresDatabaseURL(t *testing.T) {
	_, err := NewPostgresStore("", config.DefaultTimeoutConfig())
	if err == nil {
		t.Fatal("expected error for empty database URL")
	}
}
