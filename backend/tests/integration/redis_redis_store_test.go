//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/testutil"
)

func TestRedisStore_RegisterRoom_Smoke(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, rdb := testutil.SetupRedisStore(t)

	code := "ROOM1"
	data := []byte(`{"host":"Host1","players":1}`)
	if err := rdb.RegisterRoom(ctx, code, data, 5*time.Minute); err != nil {
		t.Fatalf("RegisterRoom failed: %v", err)
	}

	rooms, err := rdb.ListActiveRooms(ctx)
	if err != nil {
		t.Fatalf("ListActiveRooms failed: %v", err)
	}
	for _, r := range rooms {
		if r == code {
			return
		}
	}
	t.Fatal("registered room not found in active rooms")
}
