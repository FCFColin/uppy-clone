//go:build integration

package integration

import (
	"context"
	"sync"
	"testing"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/testutil"
)

func TestGameRoom_FullLifecycle(t *testing.T) {
	pgStore := testutil.SetupPostgres(t, testutil.WithStore(), testutil.WithMigrations()).Store
	gameStore := pgStore
	redisStore := testutil.SetupMiniredisStore(t)
	timeouts := config.DefaultTimeoutConfig()

	hub := game.NewHub(gameStore, redisStore, timeouts, 10, 50)

	ctx := context.Background()
	roomID, err := hub.CreateRoom(ctx)
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if roomID == "" {
		t.Fatal("expected non-empty room code")
	}

	info, err := hub.CheckRoom(roomID)
	if err != nil {
		t.Fatalf("CheckRoom: %v", err)
	}
	if info == nil {
		t.Fatal("expected room info")
	}
	if info.Phase != "waiting" {
		t.Fatalf("phase = %q, want waiting", info.Phase)
	}
	if info.PlayerCount != 0 {
		t.Fatalf("playerCount = %d, want 0", info.PlayerCount)
	}

	if hub.RoomCount() != 1 {
		t.Fatalf("RoomCount = %d, want 1", hub.RoomCount())
	}

	hub.CloseAllRooms()
	if hub.RoomCount() != 0 {
		t.Fatalf("RoomCount after CloseAllRooms = %d, want 0", hub.RoomCount())
	}
}

func TestGameRoom_CheckNonExistentRoom(t *testing.T) {
	pgStore := testutil.SetupPostgres(t, testutil.WithStore(), testutil.WithMigrations()).Store
	gameStore := pgStore
	redisStore := testutil.SetupMiniredisStore(t)
	timeouts := config.DefaultTimeoutConfig()

	hub := game.NewHub(gameStore, redisStore, timeouts, 10, 50)
	info, err := hub.CheckRoom("NONEXIST")
	if err != nil {
		t.Fatalf("CheckRoom: %v", err)
	}
	if info != nil {
		t.Fatal("expected nil info for non-existent room")
	}
}

func TestGameRoom_ConcurrentCreate(t *testing.T) {
	pgStore := testutil.SetupPostgres(t, testutil.WithStore(), testutil.WithMigrations()).Store
	gameStore := pgStore
	redisStore := testutil.SetupMiniredisStore(t)
	timeouts := config.DefaultTimeoutConfig()

	hub := game.NewHub(gameStore, redisStore, timeouts, 10, 50)
	ctx := context.Background()

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

	seen := make(map[string]bool)
	for code := range codes {
		if seen[code] {
			t.Fatalf("duplicate room code: %q", code)
		}
		seen[code] = true
	}
	if len(seen) != concurrency {
		t.Fatalf("expected %d rooms, got %d", concurrency, len(seen))
	}

	hub.CloseAllRooms()
}

func TestGameRoom_MatchRoom(t *testing.T) {
	pgStore := testutil.SetupPostgres(t, testutil.WithStore(), testutil.WithMigrations()).Store
	gameStore := pgStore
	redisStore := testutil.SetupMiniredisStore(t)
	timeouts := config.DefaultTimeoutConfig()

	hub := game.NewHub(gameStore, redisStore, timeouts, 10, 50)
	ctx := context.Background()

	firstCode, err := hub.MatchRoom(ctx)
	if err != nil {
		t.Fatalf("MatchRoom: %v", err)
	}
	if firstCode == "" {
		t.Fatal("expected non-empty room code")
	}

	secondCode, err := hub.MatchRoom(ctx)
	if err != nil {
		t.Fatalf("MatchRoom second: %v", err)
	}
	if secondCode == "" {
		t.Fatal("expected non-empty room code")
	}
	if secondCode != firstCode {
		t.Fatalf("MatchRoom should reuse joinable room: got %q, want %q", secondCode, firstCode)
	}

	hub.CloseAllRooms()
}

func TestGameRoom_NilStore(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	hub := game.NewHub(nil, nil, timeouts, 10, 50)
	ctx := context.Background()

	code, err := hub.CreateRoom(ctx)
	if err != nil {
		t.Fatalf("CreateRoom with nil stores: %v", err)
	}
	if code == "" {
		t.Fatal("expected non-empty code")
	}

	info, err := hub.CheckRoom(code)
	if err != nil {
		t.Fatalf("CheckRoom: %v", err)
	}
	if info == nil {
		t.Fatal("expected room info")
	}

	hub.CloseAllRooms()
}

func TestGameRoom_RemoveRoom(t *testing.T) {
	pgStore := testutil.SetupPostgres(t, testutil.WithStore(), testutil.WithMigrations()).Store
	gameStore := pgStore
	redisStore := testutil.SetupMiniredisStore(t)
	timeouts := config.DefaultTimeoutConfig()

	hub := game.NewHub(gameStore, redisStore, timeouts, 10, 50)
	ctx := context.Background()

	code, err := hub.CreateRoom(ctx)
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	if hub.RoomCount() != 1 {
		t.Fatalf("RoomCount before remove = %d, want 1", hub.RoomCount())
	}

	hub.RemoveRoom(ctx, code)
	if hub.RoomCount() != 0 {
		t.Fatalf("RoomCount after remove = %d, want 0", hub.RoomCount())
	}
}

func TestGameRoom_RemoveNonExistent(t *testing.T) {
	pgStore := testutil.SetupPostgres(t, testutil.WithStore(), testutil.WithMigrations()).Store
	gameStore := pgStore
	redisStore := testutil.SetupMiniredisStore(t)
	timeouts := config.DefaultTimeoutConfig()

	hub := game.NewHub(gameStore, redisStore, timeouts, 10, 50)
	ctx := context.Background()

	hub.RemoveRoom(ctx, "NONEXIST")
}

func TestGameRoom_CodeConflictHook(t *testing.T) {
	pgStore := testutil.SetupPostgres(t, testutil.WithStore(), testutil.WithMigrations()).Store
	gameStore := pgStore
	redisStore := testutil.SetupMiniredisStore(t)
	timeouts := config.DefaultTimeoutConfig()

	hub := game.NewHub(gameStore, redisStore, timeouts, 10, 50)
	ctx := context.Background()

	restore := hub.SetGenerateRoomCodeHook(func() string { return "AAAAA" })
	defer restore()

	code1, err := hub.CreateRoom(ctx)
	if err != nil {
		t.Fatalf("first CreateRoom: %v", err)
	}
	if code1 != "AAAAA" {
		t.Fatalf("code = %q, want AAAAA", code1)
	}

	_, err = hub.CreateRoom(ctx)
	if err == nil {
		t.Fatal("expected ErrRoomCodeConflict for duplicate code")
	}
}

func TestGameRoom_DefaultTimeoutConfig(t *testing.T) {
	pgStore := testutil.SetupPostgres(t, testutil.WithStore(), testutil.WithMigrations()).Store
	gameStore := pgStore
	redisStore := testutil.SetupMiniredisStore(t)
	timeouts := config.DefaultTimeoutConfig()

	hub := game.NewHub(gameStore, redisStore, timeouts, 10, 50)
	got := hub.Timeouts()
	if got.RedisConnectTimeout <= 0 {
		t.Fatal("expected non-zero RedisConnectTimeout")
	}
}
