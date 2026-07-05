//go:build integration

package game

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/testutil"
)

// Integration tests for Hub with real Redis via testcontainers.

func TestHubCreateRoomRedisRegistry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, rdb := testutil.SetupRedisStore(t)
	timeouts := config.DefaultTimeoutConfig()
	hub := NewHub(nil, rdb, timeouts, 0, 0, nil)

	code, err := hub.CreateRoom(ctx)
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	if code == "" {
		t.Fatal("expected non-empty room code")
	}

	// Verify room is registered in Redis.
	info, err := rdb.GetRoomRegistry(ctx, code)
	if err != nil {
		t.Fatalf("GetRoomRegistry: %v", err)
	}
	if info == nil {
		t.Fatal("room not found in Redis registry")
	}
	if info.Code != code {
		t.Fatalf("registry code = %q, want %q", info.Code, code)
	}
	if info.Instance == "" {
		t.Fatal("expected non-empty instance ID in registry")
	}

	// Verify room appears in active rooms list.
	rooms, err := rdb.ListActiveRooms(ctx)
	if err != nil {
		t.Fatalf("ListActiveRooms: %v", err)
	}
	found := false
	for _, r := range rooms {
		if r == code {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("room %q not found in active rooms list: %v", code, rooms)
	}

	// Verify the room exists in the Hub.
	if hRoom := hub.GetRoom(code); hRoom == nil {
		t.Fatal("room not found in Hub after CreateRoom")
	}
}

func TestHubRedisRestore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, rdb := testutil.SetupRedisStore(t)
	timeouts := config.DefaultTimeoutConfig()

	// Create a room in the hub.
	hub := NewHub(nil, rdb, timeouts, 0, 0, nil)
	code, err := hub.CreateRoom(ctx)
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	// Verify it was registered in Redis.
	info, err := rdb.GetRoomRegistry(ctx, code)
	if err != nil {
		t.Fatalf("GetRoomRegistry: %v", err)
	}
	if info == nil {
		t.Fatal("expected room in Redis registry")
	}

	// Create a NEW hub that will load the room from Redis.
	hub2 := NewHub(nil, rdb, timeouts, 0, 0, nil)

	// The new hub can look up the room via ResolveRoom.
	decision, err := hub2.ResolveRoom(ctx, code)
	if err != nil {
		t.Fatalf("ResolveRoom: %v", err)
	}
	if decision.Route != RouteLocal {
		t.Fatalf("expected RouteLocal for same instance, got %v", decision.Route)
	}
	if decision.Owner != hub2.instanceID {
		t.Fatalf("owner = %q, want %q", decision.Owner, hub2.instanceID)
	}

	// Register a room from a different instance.
	code2 := "RMT01"
	altRegistry := domain.RoomRegistryInfo{
		Code:      code2,
		Instance:  "remote-instance",
		Address:   "10.0.0.2:8080",
		CreatedAt: time.Now().UnixMilli(),
	}
	regData, _ := json.Marshal(altRegistry)
	if err := rdb.RegisterRoom(ctx, code2, regData, 24*time.Hour); err != nil {
		t.Fatalf("RegisterRoom: %v", err)
	}

	// Resolve room owned by remote instance.
	decision2, err := hub2.ResolveRoom(ctx, code2)
	if err != nil {
		t.Fatalf("ResolveRoom remote: %v", err)
	}
	if decision2.Route != RouteProxy {
		t.Fatalf("expected RouteProxy for remote instance, got %v", decision2.Route)
	}
	if decision2.Address != "10.0.0.2:8080" {
		t.Fatalf("address = %q, want 10.0.0.2:8080", decision2.Address)
	}
}

func TestHubCacheInvalidation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, rdb := testutil.SetupRedisStore(t)
	timeouts := config.DefaultTimeoutConfig()
	repo := newMockRoomRepository()
	hub := NewHub(repo, rdb, timeouts, 0, 0, nil)

	// Create a room — this should populate caches.
	code, err := hub.CreateRoom(ctx)
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	// Check the room to populate the room check cache.
	info, err := hub.CheckRoomCached(ctx, code)
	if err != nil {
		t.Fatalf("CheckRoomCached: %v", err)
	}
	if info == nil {
		t.Fatal("expected room info")
	}

	// Verify the room check cache is populated.
	cached, ok, err := rdb.GetCachedRoomCheck(ctx, code)
	if err != nil {
		t.Fatalf("GetCachedRoomCheck: %v", err)
	}
	if !ok || len(cached) == 0 {
		t.Fatal("expected room check cache to be populated")
	}

	// Invalidate caches.
	hub.invalidateLobbyReadCaches(code)

	// Verify room check cache is now invalidated.
	_, ok2, err := rdb.GetCachedRoomCheck(ctx, code)
	if err != nil {
		t.Fatalf("GetCachedRoomCheck after invalidation: %v", err)
	}
	if ok2 {
		t.Fatal("expected room check cache to be invalidated")
	}

	// Populate lobby list cache.
	result, err := hub.ListLobbiesCached(ctx, 10, "")
	if err != nil {
		t.Fatalf("ListLobbiesCached: %v", err)
	}
	if result == nil {
		t.Fatal("expected lobby list result")
	}

	// Verify lobby list cache is populated.
	cachedList, okList, err := rdb.GetCachedLobbyList(ctx, 10, "")
	if err != nil {
		t.Fatalf("GetCachedLobbyList: %v", err)
	}
	if !okList || len(cachedList) == 0 {
		t.Fatal("expected lobby list cache to be populated")
	}

	// Invalidate all lobby caches.
	hub.invalidateLobbyReadCaches("")

	// Verify lobby list cache is now invalidated.
	_, okList2, err := rdb.GetCachedLobbyList(ctx, 10, "")
	if err != nil {
		t.Fatalf("GetCachedLobbyList after invalidation: %v", err)
	}
	if okList2 {
		t.Fatal("expected lobby list cache to be invalidated")
	}
}

func TestHubConcurrentRoomCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, rdb := testutil.SetupRedisStore(t)
	timeouts := config.DefaultTimeoutConfig()
	hub := NewHub(nil, rdb, timeouts, 0, 0, nil)

	const concurrency = 10
	var wg sync.WaitGroup
	codes := make(chan string, concurrency)
	errCh := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, err := hub.CreateRoom(ctx)
			if err != nil {
				errCh <- err
				return
			}
			codes <- code
		}()
	}
	wg.Wait()
	close(codes)
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent CreateRoom failed: %v", err)
	}

	// Collect all created codes.
	var created []string
	for code := range codes {
		created = append(created, code)
	}

	if len(created) != concurrency {
		t.Fatalf("expected %d rooms, got %d", concurrency, len(created))
	}

	// Verify all rooms are registered in Redis.
	activeRooms, err := rdb.ListActiveRooms(ctx)
	if err != nil {
		t.Fatalf("ListActiveRooms: %v", err)
	}
	activeSet := make(map[string]bool, len(activeRooms))
	for _, r := range activeRooms {
		activeSet[r] = true
	}
	for _, code := range created {
		if !activeSet[code] {
			t.Fatalf("room %q not found in Redis active rooms", code)
		}
		if hub.GetRoom(code) == nil {
			t.Fatalf("room %q not found in Hub", code)
		}
	}

	// Verify no duplicate codes.
	seen := make(map[string]bool)
	for _, code := range created {
		if seen[code] {
			t.Fatalf("duplicate room code: %q", code)
		}
		seen[code] = true
	}
}
