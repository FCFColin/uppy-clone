package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/config"
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

func newTestSessionStore(t *testing.T) (*SessionStore, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewSessionStore(rdb), mr
}

func newTestMagicLinkStore(t *testing.T) (*MagicLinkStore, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewMagicLinkStore(rdb), mr
}

func newTestRateLimitStore(t *testing.T) (*RateLimitStore, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewRateLimitStore(rdb), mr
}

func newTestRoomRegistryStore(t *testing.T) (*RoomRegistryStore, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewRoomRegistryStore(rdb), mr
}

func newTestEmailQueueStore(t *testing.T) (*EmailQueueStore, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewEmailQueueStore(rdb), mr
}

func TestSessionStore_MagicTokenLifecycle(t *testing.T) {
	s, _ := newTestMagicLinkStore(t)
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

func TestRoomRegistryStore_TryClaimRoomRegistry(t *testing.T) {
	s, _ := newTestRoomRegistryStore(t)
	ctx := context.Background()
	code := "CLAIM1"
	payload := []byte(`{"code":"CLAIM1","instance":"inst-a","address":"addr"}`)
	ttl := time.Hour

	if err := s.RegisterRoom(ctx, code, payload, ttl); err != nil {
		t.Fatalf("RegisterRoom: %v", err)
	}

	info, err := s.GetRoomRegistry(ctx, code)
	if err != nil || info == nil {
		t.Fatalf("GetRoomRegistry = %v, %v", info, err)
	}

	if err := s.UnregisterRoom(ctx, code); err != nil {
		t.Fatalf("UnregisterRoom: %v", err)
	}
}

func TestRateLimitStore_CheckRateLimit(t *testing.T) {
	s, _ := newTestRateLimitStore(t)
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

func TestSessionStore_JWTRevocation(t *testing.T) {
	s, _ := newTestSessionStore(t)
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

func TestSessionStore_AdminLoginLockout(t *testing.T) {
	s, _ := newTestSessionStore(t)
	ctx := context.Background()
	ip := "203.0.113.1"

	ipCount, acctCount, err := s.IncrementFailedLogin(ctx, ip, "admin")
	if err != nil || ipCount != 1 || acctCount != 1 {
		t.Fatalf("IncrementFailedLogin = (%d,%d), %v", ipCount, acctCount, err)
	}
	if err := s.SetLoginLock(ctx, ip, "admin", time.Minute); err != nil {
		t.Fatalf("SetLoginLock: %v", err)
	}
	locked, err := s.IsLoginLocked(ctx, ip, "admin")
	if err != nil || !locked {
		t.Fatalf("expected locked, got %v err=%v", locked, err)
	}
	if err := s.ResetFailedLogin(ctx, ip, "admin"); err != nil {
		t.Fatalf("ResetFailedLogin: %v", err)
	}
	locked, _ = s.IsLoginLocked(ctx, ip, "admin")
	if locked {
		t.Fatal("expected lock cleared")
	}
}

func TestParseLobbyCursor(t *testing.T) {
	tests := []struct {
		cursor  string
		at      int64
		code    string
		wantErr bool
	}{
		{"", 0, "", false},
		{"1700000000|ABCD2", 1700000000, "ABCD2", false},
		{"bad|CODE", 0, "", true},    // non-numeric timestamp
		{"noseparator", 0, "", true}, // missing separator
		{"0|CODE", 0, "", true},      // zero timestamp
		{"1700000000|", 0, "", true}, // empty code
	}
	for _, tt := range tests {
		at, code, err := parseLobbyCursor(tt.cursor)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseLobbyCursor(%q) expected error, got nil", tt.cursor)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseLobbyCursor(%q) unexpected error: %v", tt.cursor, err)
			continue
		}
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

func TestRedisStore_LobbyReadCache_Errors(t *testing.T) {
	s, mr := newTestRedisStore(t)
	ctx := context.Background()

	mr.SetError("redis down")
	if _, _, err := s.GetCachedLobbyList(ctx, 10, ""); err == nil {
		t.Fatal("expected GetCachedLobbyList error")
	}
	if err := s.SetCachedLobbyList(ctx, 10, "", []byte("x")); err == nil {
		t.Fatal("expected SetCachedLobbyList error")
	}
	if _, _, err := s.GetCachedRoomCheck(ctx, "R1"); err == nil {
		t.Fatal("expected GetCachedRoomCheck error")
	}
	if err := s.SetCachedRoomCheck(ctx, "R1", []byte("x")); err == nil {
		t.Fatal("expected SetCachedRoomCheck error")
	}
	if err := s.InvalidateLobbyListCaches(ctx); err == nil {
		t.Fatal("expected InvalidateLobbyListCaches scan error")
	}
	if err := s.InvalidateRoomCheck(ctx, "R1"); err == nil {
		t.Fatal("expected InvalidateRoomCheck error")
	}
}

func TestEmailQueueStore_EnqueueStreams(t *testing.T) {
	s, mr := newTestEmailQueueStore(t)
	ctx := context.Background()
	payload := []byte(`{"to":"a@b.com"}`)

	if err := s.EnqueueEmail(ctx, payload); err != nil {
		t.Fatalf("EnqueueEmail: %v", err)
	}
	if !mr.Exists("email:queue") {
		t.Fatal("expected email:queue redis stream to exist")
	}
}

func TestGetEnvInt(t *testing.T) {
	t.Run("returns default when env not set", func(t *testing.T) {
		_ = os.Unsetenv("TEST_STORE_INT")
		got := config.GetEnvIntPositive("TEST_STORE_INT", 42)
		if got != 42 {
			t.Errorf("GetEnvIntPositive = %d, want %d", got, 42)
		}
	})
	t.Run("parses valid int", func(t *testing.T) {
		if err := os.Setenv("TEST_STORE_INT", "50"); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Unsetenv("TEST_STORE_INT") }()
		if got := config.GetEnvIntPositive("TEST_STORE_INT", 10); got != 50 {
			t.Errorf("GetEnvIntPositive = %d, want 50", got)
		}
	})
	t.Run("returns default for invalid int", func(t *testing.T) {
		if err := os.Setenv("TEST_STORE_INT", "not-a-number"); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Unsetenv("TEST_STORE_INT") }()
		if got := config.GetEnvIntPositive("TEST_STORE_INT", 10); got != 10 {
			t.Errorf("GetEnvIntPositive = %d, want 10", got)
		}
	})
}
func TestGetEnvDuration(t *testing.T) {
	t.Run("returns default when env not set", func(t *testing.T) {
		_ = os.Unsetenv("TEST_STORE_DUR")
		got := config.GetEnvDuration("TEST_STORE_DUR", 5*time.Second)
		if got != 5*time.Second {
			t.Errorf("GetEnvDuration = %v, want 5s", got)
		}
	})
	t.Run("returns default for invalid env", func(t *testing.T) {
		if err := os.Setenv("TEST_STORE_DUR", "not-a-duration"); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Unsetenv("TEST_STORE_DUR") }()
		got := config.GetEnvDuration("TEST_STORE_DUR", 5*time.Second)
		if got != 5*time.Second {
			t.Errorf("GetEnvDuration = %v, want 5s", got)
		}
	})
	t.Run("returns default for zero duration", func(t *testing.T) {
		if err := os.Setenv("TEST_STORE_DUR", "0s"); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Unsetenv("TEST_STORE_DUR") }()
		got := config.GetEnvDuration("TEST_STORE_DUR", 5*time.Second)
		if got != 5*time.Second {
			t.Errorf("GetEnvDuration = %v, want 5s", got)
		}
	})
	t.Run("parses valid duration", func(t *testing.T) {
		if err := os.Setenv("TEST_STORE_DUR", "10s"); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Unsetenv("TEST_STORE_DUR") }()
		got := config.GetEnvDuration("TEST_STORE_DUR", 5*time.Second)

		if got != 10*time.Second {
			t.Errorf("GetEnvDuration = %v, want 10s", got)
		}
	})
}

func TestSessionStore_AdminLoginLockout_RateLimitCheck(t *testing.T) {
	s, _ := newTestSessionStore(t)
	ctx := context.Background()

	ipCount, acctCount, err := s.IncrementFailedLogin(ctx, "10.0.0.1", "testadmin")
	if err != nil || ipCount != 1 || acctCount != 1 {
		t.Fatalf("IncrementFailedLogin = (%d,%d), %v", ipCount, acctCount, err)
	}
}
