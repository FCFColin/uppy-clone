package integration

import (
	"context"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/testutil"
)

func setupRedisTestStore(t *testing.T) (context.Context, *store.RedisStore) {
	t.Helper()
	return testutil.SetupRedisStore(t)
}

func testRedisRegisterAndListRooms(t *testing.T, ctx context.Context, rdb *store.RedisStore) {
	code := "ROOM1"
	data := []byte(`{"host":"Host1","players":1}`)
	ttl := 5 * time.Minute

	if err := rdb.RegisterRoom(ctx, code, data, ttl); err != nil {
		t.Fatalf("RegisterRoom failed: %v", err)
	}

	rooms, err := rdb.ListActiveRooms(ctx)
	if err != nil {
		t.Fatalf("ListActiveRooms failed: %v", err)
	}
	found := false
	for _, r := range rooms {
		if r == code {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("registered room not found in active rooms")
	}
}

func testRedisUnregisterRoom(t *testing.T, ctx context.Context, rdb *store.RedisStore) {
	code := "ROOM2"
	data := []byte(`{"host":"Host2","players":1}`)
	ttl := 5 * time.Minute

	if err := rdb.RegisterRoom(ctx, code, data, ttl); err != nil {
		t.Fatalf("RegisterRoom failed: %v", err)
	}

	if err := rdb.UnregisterRoom(ctx, code); err != nil {
		t.Fatalf("UnregisterRoom failed: %v", err)
	}

	rooms, err := rdb.ListActiveRooms(ctx)
	if err != nil {
		t.Fatalf("ListActiveRooms failed: %v", err)
	}
	for _, r := range rooms {
		if r == code {
			t.Fatal("unregistered room still found in active rooms")
		}
	}
}

// testRedisStoreAndGetMagicToken verifies StoreMagicToken, GetMagicToken,
// and DeleteMagicToken lifecycle.
func testRedisStoreAndGetMagicToken(t *testing.T, ctx context.Context, rdb *store.RedisStore) {
	token := "hashed-token-123"
	data := []byte(`{"email":"test@example.com"}`)
	ttl := 5 * time.Minute

	if err := rdb.StoreMagicToken(ctx, token, data, ttl); err != nil {
		t.Fatalf("StoreMagicToken failed: %v", err)
	}

	got, err := rdb.GetMagicToken(ctx, token)
	if err != nil {
		t.Fatalf("GetMagicToken failed: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("expected %s, got %s", string(data), string(got))
	}

	if err := rdb.DeleteMagicToken(ctx, token); err != nil {
		t.Fatalf("DeleteMagicToken failed: %v", err)
	}

	got2, err := rdb.GetMagicToken(ctx, token)
	if err != nil {
		t.Fatalf("GetMagicToken after delete failed: %v", err)
	}
	if got2 != nil {
		t.Fatal("expected nil after deleting magic token")
	}
}

// testRedisCheckRateLimit verifies the rate limiter allows up to maxCount
// requests and blocks the next one.
func testRedisCheckRateLimit(t *testing.T, ctx context.Context, rdb *store.RedisStore) {
	key := "rate-test-integration"
	maxCount := int64(3)
	window := 10 * time.Second

	for i := 0; i < 3; i++ {
		allowed, err := rdb.CheckRateLimit(ctx, key, maxCount, window)
		if err != nil {
			t.Fatalf("CheckRateLimit failed: %v", err)
		}
		if !allowed {
			t.Fatalf("expected request %d to be allowed", i+1)
		}
	}

	allowed, err := rdb.CheckRateLimit(ctx, key, maxCount, window)
	if err != nil {
		t.Fatalf("CheckRateLimit failed: %v", err)
	}
	if allowed {
		t.Fatal("expected 4th request to be rate limited")
	}
}
