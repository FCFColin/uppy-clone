package integration

import (
	"testing"
)

func TestRedisStore_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, rdb := setupRedisTestStore(t)

	t.Run("RegisterAndListRooms", func(t *testing.T) {
		testRedisRegisterAndListRooms(t, ctx, rdb)
	})

	t.Run("UnregisterRoom", func(t *testing.T) {
		testRedisUnregisterRoom(t, ctx, rdb)
	})

	t.Run("StoreAndGetMagicToken", func(t *testing.T) {
		testRedisStoreAndGetMagicToken(t, ctx, rdb)
	})

	t.Run("CheckRateLimit", func(t *testing.T) {
		testRedisCheckRateLimit(t, ctx, rdb)
	})
}
