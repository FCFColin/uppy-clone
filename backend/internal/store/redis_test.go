package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/domain"
)

func TestRedisKeyHelpers(t *testing.T) {
	t.Parallel()

	t.Run("jwtRevokedKey", func(t *testing.T) {
		got := jwtRevokedKey("test-jti")
		if got != "jwt_revoked:test-jti" {
			t.Errorf("jwtRevokedKey = %q, want %q", got, "jwt_revoked:test-jti")
		}
	})

	t.Run("magicTokenKey", func(t *testing.T) {
		got := magicTokenKey("abc123")
		if got != "magic:abc123" {
			t.Errorf("magicTokenKey = %q, want %q", got, "magic:abc123")
		}
	})

	t.Run("roomInfoKey", func(t *testing.T) {
		got := roomInfoKey("ABCD1")
		if got != "room:ABCD1" {
			t.Errorf("roomInfoKey = %q, want %q", got, "room:ABCD1")
		}
	})

	t.Run("rateLimitKey", func(t *testing.T) {
		got := rateLimitKey("user:123")
		if got != "rl:user:123" {
			t.Errorf("rateLimitKey = %q, want %q", got, "rl:user:123")
		}
	})

	t.Run("lobbyListCacheKey", func(t *testing.T) {
		got := lobbyListCacheKey(20, "cursor123")
		if got != "lobby:list:20:cursor123" {
			t.Errorf("lobbyListCacheKey = %q, want %q", got, "lobby:list:20:cursor123")
		}
	})

	t.Run("lobbyCheckCacheKey", func(t *testing.T) {
		got := lobbyCheckCacheKey("ABCD1")
		if got != "lobby:check:ABCD1" {
			t.Errorf("lobbyCheckCacheKey = %q, want %q", got, "lobby:check:ABCD1")
		}
	})
}

func newTestRedisStore(t *testing.T) (*RedisStore, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewRedisStoreFromClient(rdb), mr
}

func TestRedisStore_MagicTokenLifecycle(t *testing.T) {
	s, _ := newTestRedisStore(t)
	ctx := context.Background()
	data := []byte(`{"email":"a@b.com"}`)
	hash := "abc123"

	if err := s.StoreMagicToken(ctx, hash, data, time.Minute); err != nil {
		t.Fatalf("StoreMagicToken: %v", err)
	}
	got, err := s.GetMagicToken(ctx, hash)
	if err != nil || string(got) != string(data) {
		t.Fatalf("GetMagicToken = %q, %v", got, err)
	}
	if err := s.DeleteMagicToken(ctx, hash); err != nil {
		t.Fatalf("DeleteMagicToken: %v", err)
	}
	missing, err := s.GetMagicToken(ctx, hash)
	if err != nil || missing != nil {
		t.Fatalf("expected nil after delete, got %q err=%v", missing, err)
	}
}

func TestRedisStore_RoomRegistry(t *testing.T) {
	s, _ := newTestRedisStore(t)
	ctx := context.Background()
	code := "ABCD2"
	payload := []byte(`{"players":1}`)

	if err := s.RegisterRoom(ctx, code, payload, time.Hour); err != nil {
		t.Fatalf("RegisterRoom: %v", err)
	}
	rooms, err := s.ListActiveRooms(ctx)
	if err != nil || len(rooms) != 1 || rooms[0] != code {
		t.Fatalf("ListActiveRooms = %v, %v", rooms, err)
	}
	if err := s.UnregisterRoom(ctx, code); err != nil {
		t.Fatalf("UnregisterRoom: %v", err)
	}
	rooms, _ = s.ListActiveRooms(ctx)
	if len(rooms) != 0 {
		t.Fatalf("expected empty rooms, got %v", rooms)
	}
}

func TestRedisStore_CheckRateLimit(t *testing.T) {
	s, _ := newTestRedisStore(t)
	ctx := context.Background()
	key := "auth:login:127.0.0.1"

	for i := 0; i < 3; i++ {
		allowed, err := s.CheckRateLimit(ctx, key, 3, time.Minute)
		if err != nil || !allowed {
			t.Fatalf("request %d: allowed=%v err=%v", i+1, allowed, err)
		}
	}
	// Adversarial: 4th request within window must be denied.
	allowed, err := s.CheckRateLimit(ctx, key, 3, time.Minute)
	if err != nil || allowed {
		t.Fatalf("expected rate limit deny, allowed=%v err=%v", allowed, err)
	}
}

func TestRedisStore_JWTRevocation(t *testing.T) {
	s, _ := newTestRedisStore(t)
	ctx := context.Background()
	jti := "token-jti-1"

	revoked, err := s.IsJWTRevoked(ctx, jti)
	if err != nil || revoked {
		t.Fatalf("fresh jti should not be revoked: %v %v", revoked, err)
	}
	if err := s.RevokeJWT(ctx, jti, time.Minute); err != nil {
		t.Fatalf("RevokeJWT: %v", err)
	}
	revoked, err = s.IsJWTRevoked(ctx, jti)
	if err != nil || !revoked {
		t.Fatalf("expected revoked=true, got %v err=%v", revoked, err)
	}
}

func TestRedisStore_AdminLoginLockout(t *testing.T) {
	s, _ := newTestRedisStore(t)
	ctx := context.Background()
	ip := "203.0.113.1"

	count, err := s.IncrementFailedLogin(ctx, ip)
	if err != nil || count != 1 {
		t.Fatalf("IncrementFailedLogin = %d, %v", count, err)
	}
	if err := s.SetLoginLock(ctx, ip, time.Minute); err != nil {
		t.Fatalf("SetLoginLock: %v", err)
	}
	locked, err := s.IsLoginLocked(ctx, ip)
	if err != nil || !locked {
		t.Fatalf("expected locked, got %v err=%v", locked, err)
	}
	if err := s.ResetFailedLogin(ctx, ip); err != nil {
		t.Fatalf("ResetFailedLogin: %v", err)
	}
	locked, _ = s.IsLoginLocked(ctx, ip)
	if locked {
		t.Fatal("expected lock cleared")
	}
}

func TestParseLobbyCursor(t *testing.T) {
	tests := []struct {
		cursor string
		at     int64
		code   string
	}{
		{"", 0, ""},
		{"1700000000|ABCD2", 1700000000, "ABCD2"},
		{"bad|CODE", 0, "CODE"},
	}
	for _, tt := range tests {
		at, code := parseLobbyCursor(tt.cursor)
		if at != tt.at || code != tt.code {
			t.Errorf("parseLobbyCursor(%q) = (%d,%q), want (%d,%q)", tt.cursor, at, code, tt.at, tt.code)
		}
	}
}

func TestBuildLobbyListResult_Pagination(t *testing.T) {
	lobbies := []domain.LobbyState{
		{Code: "A", UpdatedAt: 3},
		{Code: "B", UpdatedAt: 2},
		{Code: "C", UpdatedAt: 1},
	}
	result := buildLobbyListResult(lobbies, 10, 2)
	if !result.HasMore || len(result.Lobbies) != 2 {
		t.Fatalf("HasMore=%v len=%d", result.HasMore, len(result.Lobbies))
	}
	if result.NextCursor != "2|B" {
		t.Errorf("NextCursor = %q", result.NextCursor)
	}
}

func TestRedisStore_LobbyReadCache(t *testing.T) {
	s, _ := newTestRedisStore(t)
	ctx := context.Background()

	t.Run("miss then hit lobby list cache", func(t *testing.T) {
		got, hit, err := s.GetCachedLobbyList(ctx, 20, "")
		if err != nil || hit || got != nil {
			t.Fatalf("initial miss expected: got=%v hit=%v err=%v", got, hit, err)
		}
		data := []byte(`[{"code":"ABC"}]`)
		if err := s.SetCachedLobbyList(ctx, 20, "", data); err != nil {
			t.Fatalf("SetCachedLobbyList: %v", err)
		}
		got, hit, err = s.GetCachedLobbyList(ctx, 20, "")
		if err != nil || !hit || string(got) != string(data) {
			t.Fatalf("after set: got=%q hit=%v err=%v", got, hit, err)
		}
	})

	t.Run("miss then hit room check cache", func(t *testing.T) {
		got, hit, err := s.GetCachedRoomCheck(ctx, "ABCD1")
		if err != nil || hit || got != nil {
			t.Fatalf("initial miss expected: got=%v hit=%v err=%v", got, hit, err)
		}
		data := []byte(`{"exists":true}`)
		if err := s.SetCachedRoomCheck(ctx, "ABCD1", data); err != nil {
			t.Fatalf("SetCachedRoomCheck: %v", err)
		}
		got, hit, err = s.GetCachedRoomCheck(ctx, "ABCD1")
		if err != nil || !hit || string(got) != string(data) {
			t.Fatalf("after set: got=%q hit=%v err=%v", got, hit, err)
		}
	})

	t.Run("invalidate single room check", func(t *testing.T) {
		if err := s.SetCachedRoomCheck(ctx, "ROOM1", []byte(`{"x":1}`)); err != nil {
			t.Fatal(err)
		}
		if err := s.InvalidateRoomCheck(ctx, "ROOM1"); err != nil {
			t.Fatalf("InvalidateRoomCheck: %v", err)
		}
		_, hit, err := s.GetCachedRoomCheck(ctx, "ROOM1")
		if err != nil || hit {
			t.Fatalf("expected miss after invalidate: hit=%v err=%v", hit, err)
		}
	})

	t.Run("invalidate all lobby list caches", func(t *testing.T) {
		if err := s.SetCachedLobbyList(ctx, 10, "a", []byte(`[1]`)); err != nil {
			t.Fatal(err)
		}
		if err := s.SetCachedLobbyList(ctx, 20, "b", []byte(`[2]`)); err != nil {
			t.Fatal(err)
		}
		if err := s.InvalidateLobbyListCaches(ctx); err != nil {
			t.Fatalf("InvalidateLobbyListCaches: %v", err)
		}
		_, hit1, _ := s.GetCachedLobbyList(ctx, 10, "a")
		_, hit2, _ := s.GetCachedLobbyList(ctx, 20, "b")
		if hit1 || hit2 {
			t.Fatalf("expected both misses after invalidate, hit1=%v hit2=%v", hit1, hit2)
		}
	})
}

func TestRedisStore_RoomRegistryInfo(t *testing.T) {
	s, _ := newTestRedisStore(t)
	ctx := context.Background()

	t.Run("get non-existent returns nil", func(t *testing.T) {
		info, err := s.GetRoomRegistry(ctx, "NONEXIST")
		if err != nil || info != nil {
			t.Fatalf("GetRoomRegistry = %v, %v (want nil, nil)", info, err)
		}
	})

	t.Run("set and get room registry", func(t *testing.T) {
		code := "ABCD1"
		if err := s.RegisterRoom(ctx, code, []byte(`{"code":"ABCD1","instance":"i1","address":"addr1","created_at":100}`), time.Hour); err != nil {
			t.Fatalf("RegisterRoom: %v", err)
		}
		info, err := s.GetRoomRegistry(ctx, code)
		if err != nil || info == nil {
			t.Fatalf("GetRoomRegistry = %v, %v", info, err)
		}
		if info.Code != "ABCD1" || info.Instance != "i1" {
			t.Errorf("RoomRegistryInfo = %+v", info)
		}
	})

	t.Run("renew room registry extends TTL", func(t *testing.T) {
		code := "RENEW1"
		if err := s.RegisterRoom(ctx, code, []byte(`{"code":"RENEW1","instance":"i2","address":"addr2"}`), time.Second); err != nil {
			t.Fatalf("RegisterRoom: %v", err)
		}
		if err := s.RenewRoomRegistry(ctx, code, time.Hour); err != nil {
			t.Fatalf("RenewRoomRegistry: %v", err)
		}
		info, err := s.GetRoomRegistry(ctx, code)
		if err != nil || info == nil {
			t.Fatalf("GetRoomRegistry after renew = %v, %v", info, err)
		}
	})
}

func TestRedisStore_EnqueueStreams(t *testing.T) {
	s, mr := newTestRedisStore(t)
	ctx := context.Background()
	payload := []byte(`{"to":"a@b.com"}`)

	if err := s.EnqueueEmail(ctx, payload); err != nil {
		t.Fatalf("EnqueueEmail: %v", err)
	}
	if err := s.EnqueueGameResult(ctx, payload); err != nil {
		t.Fatalf("EnqueueGameResult: %v", err)
	}
	if !mr.Exists("email:queue") || !mr.Exists("game:results") {
		t.Fatal("expected redis streams to exist")
	}
}

func TestGetEnvInt(t *testing.T) {
	t.Run("returns default when env not set", func(t *testing.T) {
		os.Unsetenv("TEST_STORE_INT")
		got := getEnvInt("TEST_STORE_INT", 42)
		if got != 42 {
			t.Errorf("getEnvInt = %d, want %d", got, 42)
		}
	})
	t.Run("parses valid int", func(t *testing.T) {
		os.Setenv("TEST_STORE_INT", "50")
		defer os.Unsetenv("TEST_STORE_INT")
		if got := getEnvInt("TEST_STORE_INT", 10); got != 50 {
			t.Errorf("getEnvInt = %d, want 50", got)
		}
	})
}

func TestGetEnvDuration(t *testing.T) {
	t.Run("returns default when env not set", func(t *testing.T) {
		os.Unsetenv("TEST_STORE_DUR")
		got := getEnvDuration("TEST_STORE_DUR", 5*time.Second)
		if got != 5*time.Second {
			t.Errorf("getEnvDuration = %v, want 5s", got)
		}
	})
}
