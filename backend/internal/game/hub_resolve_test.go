package game

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/store"
)

func TestHub_ResolveRoom_LocalWhenOwnerMatches(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	redisStore, err := store.NewRedisStore(mr.Addr(), config.DefaultTimeoutConfig())
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}

	t.Setenv("INSTANCE_ID", "instance-a")
	h := NewHub(nil, redisStore, config.DefaultTimeoutConfig(), 0, 0, nil)

	ctx := context.Background()
	code := "ABCDE"
	info, _ := json.Marshal(store.RoomRegistryInfo{
		Code:      code,
		Instance:  "instance-a",
		Address:   "10.0.0.1:8080",
		CreatedAt: time.Now().UnixMilli(),
	})
	if err := redisStore.RegisterRoom(ctx, code, info, time.Hour); err != nil {
		t.Fatalf("RegisterRoom: %v", err)
	}

	decision, err := h.ResolveRoom(ctx, code)
	if err != nil {
		t.Fatalf("ResolveRoom: %v", err)
	}
	if decision.Route != RouteLocal {
		t.Fatalf("Route = %v, want RouteLocal", decision.Route)
	}
}

func TestHub_ResolveRoom_ProxyWhenOwnerDiffers(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	redisStore, err := store.NewRedisStore(mr.Addr(), config.DefaultTimeoutConfig())
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}

	t.Setenv("INSTANCE_ID", "instance-b")
	h := NewHub(nil, redisStore, config.DefaultTimeoutConfig(), 0, 0, nil)

	ctx := context.Background()
	code := "FGHIJ"
	info, _ := json.Marshal(store.RoomRegistryInfo{
		Code:      code,
		Instance:  "instance-a",
		Address:   "10.0.0.2:8080",
		CreatedAt: time.Now().UnixMilli(),
	})
	if err := redisStore.RegisterRoom(ctx, code, info, time.Hour); err != nil {
		t.Fatalf("RegisterRoom: %v", err)
	}

	decision, err := h.ResolveRoom(ctx, code)
	if err != nil {
		t.Fatalf("ResolveRoom: %v", err)
	}
	if decision.Route != RouteProxy {
		t.Fatalf("Route = %v, want RouteProxy", decision.Route)
	}
	if decision.Address != "10.0.0.2:8080" {
		t.Fatalf("Address = %q", decision.Address)
	}
}

func TestInstanceAddress_DefaultsToLocalhostPort(t *testing.T) {
	os.Unsetenv("INSTANCE_ADDR")
	os.Unsetenv("PORT")
	if got := instanceAddress(); got != "127.0.0.1:8080" {
		t.Fatalf("instanceAddress() = %q", got)
	}
}
