package game

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/uppy-clone/backend/internal/config"
)

func TestInstanceAddress(t *testing.T) {
	t.Run("uses INSTANCE_ADDR when set", func(t *testing.T) {
		os.Setenv("INSTANCE_ADDR", "10.0.0.1:9000")
		defer os.Unsetenv("INSTANCE_ADDR")
		addr := instanceAddress()
		if addr != "10.0.0.1:9000" {
			t.Errorf("instanceAddress = %q, want %q", addr, "10.0.0.1:9000")
		}
	})

	t.Run("falls back to PORT when INSTANCE_ADDR empty", func(t *testing.T) {
		os.Unsetenv("INSTANCE_ADDR")
		os.Setenv("PORT", "3000")
		defer os.Unsetenv("PORT")
		addr := instanceAddress()
		if addr != "127.0.0.1:3000" {
			t.Errorf("instanceAddress = %q, want %q", addr, "127.0.0.1:3000")
		}
	})

	t.Run("defaults to 8080 when nothing set", func(t *testing.T) {
		os.Unsetenv("INSTANCE_ADDR")
		os.Unsetenv("PORT")
		addr := instanceAddress()
		if addr != "127.0.0.1:8080" {
			t.Errorf("instanceAddress = %q, want %q", addr, "127.0.0.1:8080")
		}
	})

	t.Run("returns address starting with 127.0.0.1", func(t *testing.T) {
		os.Unsetenv("INSTANCE_ADDR")
		os.Unsetenv("PORT")
		addr := instanceAddress()
		if !strings.HasPrefix(addr, "127.0.0.1:") {
			t.Errorf("instanceAddress = %q, want 127.0.0.1:… prefix", addr)
		}
	})
}

func TestResolveRoom_NilRedis(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0, nil)
	if h.redis != nil {
		t.Fatal("expected nil redis")
	}

	decision, err := h.ResolveRoom(context.Background(), "ABCD1")
	if err != nil {
		t.Fatalf("ResolveRoom error: %v", err)
	}
	if decision.Route != RouteLocal {
		t.Errorf("Route = %d, want RouteLocal", decision.Route)
	}
}

func TestInvalidateLobbyReadCaches_NilRedis(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0, nil)

	// Should not panic when redis is nil
	h.invalidateLobbyReadCaches("ABCD1")
	h.invalidateLobbyReadCaches("")
}
