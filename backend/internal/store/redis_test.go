package store

import (
	"context"
	"fmt"
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

type redisCmdHook struct {
	mr     *miniredis.Miniredis
	failOn map[string]error
	before map[string]func(*miniredis.Miniredis, redis.Cmder) error
}

func (h redisCmdHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h redisCmdHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if fn, ok := h.before[cmd.Name()]; ok {
			if err := fn(h.mr, cmd); err != nil {
				return err
			}
		}
		if err, ok := h.failOn[cmd.Name()]; ok {
			return err
		}
		return next(ctx, cmd)
	}
}

func (h redisCmdHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			if err, ok := h.failOn[cmd.Name()]; ok {
				return err
			}
		}
		return next(ctx, cmds)
	}
}

func newTestRedisStoreWithHook(t *testing.T, failOn map[string]error) (*RedisStore, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	rdb.AddHook(redisCmdHook{mr: mr, failOn: failOn})
	return NewRedisStoreFromClient(rdb), mr
}

func isSetNXCmd(cmd redis.Cmder) bool {
	for _, arg := range cmd.Args()[1:] {
		if fmt.Sprint(arg) == "nx" {
			return true
		}
	}
	return false
}

func newTestRedisStoreWithBeforeHook(t *testing.T, before map[string]func(*miniredis.Miniredis, redis.Cmder) error) (*RedisStore, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	rdb.AddHook(redisCmdHook{mr: mr, before: before})
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

func TestRedisStore_TryClaimRoomRegistry(t *testing.T) {
	s, _ := newTestRedisStore(t)
	ctx := context.Background()
	code := "CLAIM1"
	payload := []byte(`{"code":"CLAIM1","instance":"inst-a","address":"addr"}`)
	ttl := time.Hour

	ok, err := s.TryClaimRoomRegistry(ctx, code, payload, "inst-a", ttl)
	if err != nil || !ok {
		t.Fatalf("first claim: ok=%v err=%v", ok, err)
	}

	ok, err = s.TryClaimRoomRegistry(ctx, code, payload, "inst-b", ttl)
	if err != nil || ok {
		t.Fatalf("other instance should not claim: ok=%v err=%v", ok, err)
	}

	ok, err = s.TryClaimRoomRegistry(ctx, code, payload, "inst-a", ttl)
	if err != nil || !ok {
		t.Fatalf("same instance renew: ok=%v err=%v", ok, err)
	}
}

func TestRedisStore_TryClaimRoomRegistry_ConcurrentSetNX(t *testing.T) {
	s, _ := newTestRedisStore(t)
	ctx := context.Background()
	code := "RACE1"
	payload := []byte(`{"code":"RACE1","instance":"inst-a"}`)

	if err := s.RegisterRoom(ctx, code, payload, time.Hour); err != nil {
		t.Fatalf("RegisterRoom: %v", err)
	}

	ok, err := s.TryClaimRoomRegistry(ctx, code, []byte(`{"code":"RACE1","instance":"inst-a"}`), "inst-a", time.Hour)
	if err != nil || !ok {
		t.Fatalf("existing owner reclaim: ok=%v err=%v", ok, err)
	}
}

func TestRedisStore_TryClaimRoomRegistry_SetNXLostToOtherInstance(t *testing.T) {
	s, mr := newTestRedisStore(t)
	ctx := context.Background()
	code := "RACE2"
	otherPayload := []byte(`{"code":"RACE2","instance":"inst-b","address":"b"}`)
	mr.Set("room:"+code, string(otherPayload))

	ok, err := s.TryClaimRoomRegistry(ctx, code, []byte(`{"code":"RACE2","instance":"inst-a"}`), "inst-a", time.Hour)
	if err != nil {
		t.Fatalf("TryClaimRoomRegistry: %v", err)
	}
	if ok {
		t.Fatal("expected claim failure when another instance owns room")
	}
}

func TestRedisStore_TryClaimRoomRegistry_GetError(t *testing.T) {
	s, mr := newTestRedisStore(t)
	mr.SetError("redis down")
	_, err := s.TryClaimRoomRegistry(context.Background(), "ERR1", nil, "inst-a", time.Hour)
	if err == nil {
		t.Fatal("expected GetRoomRegistry error")
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
		got := config.GetEnvIntPositive("TEST_STORE_INT", 42)
		if got != 42 {
			t.Errorf("GetEnvIntPositive = %d, want %d", got, 42)
		}
	})
	t.Run("parses valid int", func(t *testing.T) {
		os.Setenv("TEST_STORE_INT", "50")
		defer os.Unsetenv("TEST_STORE_INT")
		if got := config.GetEnvIntPositive("TEST_STORE_INT", 10); got != 50 {
			t.Errorf("GetEnvIntPositive = %d, want 50", got)
		}
	})
	t.Run("returns default for invalid int", func(t *testing.T) {
		os.Setenv("TEST_STORE_INT", "not-a-number")
		defer os.Unsetenv("TEST_STORE_INT")
		if got := config.GetEnvIntPositive("TEST_STORE_INT", 10); got != 10 {
			t.Errorf("GetEnvIntPositive = %d, want 10", got)
		}
	})
}
func TestGetEnvDuration(t *testing.T) {
	t.Run("returns default when env not set", func(t *testing.T) {
		os.Unsetenv("TEST_STORE_DUR")
		got := config.GetEnvDuration("TEST_STORE_DUR", 5*time.Second)
		if got != 5*time.Second {
			t.Errorf("GetEnvDuration = %v, want 5s", got)
		}
	})
	t.Run("returns default for invalid env", func(t *testing.T) {
		os.Setenv("TEST_STORE_DUR", "not-a-duration")
		defer os.Unsetenv("TEST_STORE_DUR")
		got := config.GetEnvDuration("TEST_STORE_DUR", 5*time.Second)
		if got != 5*time.Second {
			t.Errorf("GetEnvDuration = %v, want 5s", got)
		}
	})
	t.Run("returns default for zero duration", func(t *testing.T) {
		os.Setenv("TEST_STORE_DUR", "0s")
		defer os.Unsetenv("TEST_STORE_DUR")
		got := config.GetEnvDuration("TEST_STORE_DUR", 5*time.Second)
		if got != 5*time.Second {
			t.Errorf("GetEnvDuration = %v, want 5s", got)
		}
	})
	t.Run("parses valid duration", func(t *testing.T) {
		os.Setenv("TEST_STORE_DUR", "10s")
		defer os.Unsetenv("TEST_STORE_DUR")
		got := config.GetEnvDuration("TEST_STORE_DUR", 5*time.Second)

		if got != 10*time.Second {
			t.Errorf("GetEnvDuration = %v, want 10s", got)
		}
	})
}

func TestRedisStore_ListActiveRooms_Error(t *testing.T) {
	s, mr := newTestRedisStore(t)
	ctx := context.Background()
	mr.SetError("redis unavailable")

	rooms, err := s.ListActiveRooms(ctx)
	if err == nil {
		t.Fatal("expected error from ListActiveRooms")
	}
	if rooms != nil {
		t.Fatalf("expected nil rooms on error, got %v", rooms)
	}
}

func TestRedisStore_TryClaimRoomRegistry_SetNXError(t *testing.T) {
	s, _ := newTestRedisStoreWithHook(t, map[string]error{"set": fmt.Errorf("setnx failed")})
	_, err := s.TryClaimRoomRegistry(context.Background(), "ERR2", []byte(`{}`), "inst-a", time.Hour)
	if err == nil {
		t.Fatal("expected SetNX error")
	}
}

func TestRedisStore_InvalidateLobbyListCaches_ScanError(t *testing.T) {
	s, mr := newTestRedisStore(t)
	mr.SetError("scan failed")
	if err := s.InvalidateLobbyListCaches(context.Background()); err == nil {
		t.Fatal("expected scan error")
	}
}

func TestRedisStore_TryClaimRoomRegistry_SAddError(t *testing.T) {
	s, _ := newTestRedisStoreWithHook(t, map[string]error{"sadd": fmt.Errorf("sadd failed")})
	ok, err := s.TryClaimRoomRegistry(context.Background(), "SADD1", []byte(`{"code":"SADD1"}`), "inst-a", time.Hour)
	if err == nil || ok {
		t.Fatalf("expected SAdd error, ok=%v err=%v", ok, err)
	}
}

func TestRedisStore_TryClaimRoomRegistry_LostSetNXRace(t *testing.T) {
	code := "RACE3"
	otherPayload := `{"code":"RACE3","instance":"inst-b","address":"b"}`
	s, _ := newTestRedisStoreWithBeforeHook(t, map[string]func(*miniredis.Miniredis, redis.Cmder) error{
		"set": func(mr *miniredis.Miniredis, cmd redis.Cmder) error {
			if isSetNXCmd(cmd) {
				mr.Set("room:"+code, otherPayload)
			}
			return nil
		},
	})

	ok, err := s.TryClaimRoomRegistry(context.Background(), code, []byte(`{"code":"RACE3","instance":"inst-a"}`), "inst-a", time.Hour)
	if err != nil {
		t.Fatalf("TryClaimRoomRegistry: %v", err)
	}
	if ok {
		t.Fatal("expected lost SetNX race to return false")
	}
}

func TestRedisStore_TryClaimRoomRegistry_SecondGetError(t *testing.T) {
	code := "NX2"
	otherPayload := `{"code":"NX2","instance":"inst-b"}`
	gets := 0
	s, _ := newTestRedisStoreWithBeforeHook(t, map[string]func(*miniredis.Miniredis, redis.Cmder) error{
		"get": func(_ *miniredis.Miniredis, _ redis.Cmder) error {
			gets++
			if gets == 2 {
				return fmt.Errorf("get failed after setnx race")
			}
			return nil
		},
		"set": func(mr *miniredis.Miniredis, cmd redis.Cmder) error {
			if isSetNXCmd(cmd) {
				mr.Set("room:"+code, otherPayload)
			}
			return nil
		},
	})

	_, err := s.TryClaimRoomRegistry(context.Background(), code, []byte(`{}`), "inst-a", time.Hour)
	if err == nil {
		t.Fatal("expected second GetRoomRegistry error")
	}
}

func TestRedisStore_InvalidateLobbyListCaches_DelError(t *testing.T) {
	s, _ := newTestRedisStoreWithHook(t, map[string]error{"del": fmt.Errorf("del failed")})
	ctx := context.Background()
	if err := s.SetCachedLobbyList(ctx, 10, "", []byte(`[1]`)); err != nil {
		t.Fatal(err)
	}
	if err := s.InvalidateLobbyListCaches(ctx); err == nil {
		t.Fatal("expected del error")
	}
}

func TestRedisStore_InvalidateRoomCheck_Error(t *testing.T) {
	s, mr := newTestRedisStore(t)
	mr.SetError("del failed")
	if err := s.InvalidateRoomCheck(context.Background(), "ROOMX"); err == nil {
		t.Fatal("expected invalidate error")
	}
}

func TestRedisStore_GetCachedLobbyList_Error(t *testing.T) {
	s, mr := newTestRedisStore(t)
	mr.SetError("get failed")
	_, _, err := s.GetCachedLobbyList(context.Background(), 10, "")
	if err == nil {
		t.Fatal("expected get error")
	}
}

func TestRedisStore_SetCachedLobbyList_Error(t *testing.T) {
	s, mr := newTestRedisStore(t)
	mr.SetError("set failed")
	if err := s.SetCachedLobbyList(context.Background(), 10, "", []byte(`[]`)); err == nil {
		t.Fatal("expected set error")
	}
}

func TestRedisStore_GetCachedRoomCheck_Error(t *testing.T) {
	s, mr := newTestRedisStore(t)
	mr.SetError("get failed")
	_, _, err := s.GetCachedRoomCheck(context.Background(), "ABCD1")
	if err == nil {
		t.Fatal("expected get error")
	}
}

func TestRedisStore_SetCachedRoomCheck_Error(t *testing.T) {
	s, mr := newTestRedisStore(t)
	mr.SetError("set failed")
	if err := s.SetCachedRoomCheck(context.Background(), "ABCD1", []byte(`{}`)); err == nil {
		t.Fatal("expected set error")
	}
}

func TestRedisStore_EnqueueEmail_Error(t *testing.T) {
	s, mr := newTestRedisStore(t)
	mr.SetError("redis down")
	if err := s.EnqueueEmail(context.Background(), []byte(`{}`)); err == nil {
		t.Fatal("expected EnqueueEmail error")
	}
}

func TestRedisStore_EnqueueGameResult_Error(t *testing.T) {
	s, mr := newTestRedisStore(t)
	mr.SetError("redis down")
	if err := s.EnqueueGameResult(context.Background(), []byte(`{}`)); err == nil {
		t.Fatal("expected EnqueueGameResult error")
	}
}

func TestRedisStore_RevokeJWT_Error(t *testing.T) {
	s, mr := newTestRedisStore(t)
	mr.SetError("redis down")
	if err := s.RevokeJWT(context.Background(), "jti-1", time.Minute); err == nil {
		t.Fatal("expected RevokeJWT error")
	}
}

func TestRedisStore_IsJWTRevoked_GetError(t *testing.T) {
	s, mr := newTestRedisStore(t)
	mr.SetError("redis down")
	_, err := s.IsJWTRevoked(context.Background(), "jti-1")
	if err == nil {
		t.Fatal("expected IsJWTRevoked error")
	}
}

func TestRedisStore_IncrementFailedLogin_Error(t *testing.T) {
	s, mr := newTestRedisStore(t)
	mr.SetError("redis down")
	_, _, err := s.IncrementFailedLogin(context.Background(), "1.2.3.4", "admin")
	if err == nil {
		t.Fatal("expected IncrementFailedLogin error")
	}
}

func TestRedisStore_IsLoginLocked_Error(t *testing.T) {
	s, mr := newTestRedisStore(t)
	mr.SetError("redis down")
	_, err := s.IsLoginLocked(context.Background(), "1.2.3.4", "admin")
	if err == nil {
		t.Fatal("expected IsLoginLocked error")
	}
}

func TestRedisStore_SetLoginLock_Error(t *testing.T) {
	s, mr := newTestRedisStore(t)
	mr.SetError("redis down")
	if err := s.SetLoginLock(context.Background(), "1.2.3.4", "admin", time.Minute); err == nil {
		t.Fatal("expected SetLoginLock error")
	}
}

func TestRedisStore_StoreMagicToken_Error(t *testing.T) {
	s, mr := newTestRedisStore(t)
	mr.SetError("redis down")
	if err := s.StoreMagicToken(context.Background(), "hash", []byte("x"), time.Minute); err == nil {
		t.Fatal("expected StoreMagicToken error")
	}
}

func TestRedisStore_GetMagicToken_GetError(t *testing.T) {
	s, mr := newTestRedisStore(t)
	mr.SetError("redis down")
	_, err := s.GetMagicToken(context.Background(), "hash")
	if err == nil {
		t.Fatal("expected GetMagicToken error")
	}
}

func TestRedisStore_DeleteMagicToken_Error(t *testing.T) {
	s, mr := newTestRedisStore(t)
	mr.SetError("redis down")
	if err := s.DeleteMagicToken(context.Background(), "hash"); err == nil {
		t.Fatal("expected DeleteMagicToken error")
	}
}

func TestRedisStore_CheckRateLimit_ScriptError(t *testing.T) {
	s, mr := newTestRedisStore(t)
	mr.SetError("redis down")
	_, err := s.CheckRateLimit(context.Background(), "k", 3, time.Minute)
	if err == nil {
		t.Fatal("expected CheckRateLimit error")
	}
}

func TestRedisStore_CheckRateLimit_UnexpectedResult(t *testing.T) {
	s, _ := newTestRedisStore(t)
	orig := rateLimitScript
	t.Cleanup(func() { rateLimitScript = orig })
	rateLimitScript = redis.NewScript(`return "bad"`)

	_, err := s.CheckRateLimit(context.Background(), "k", 3, time.Minute)
	if err == nil {
		t.Fatal("expected unexpected result error")
	}
}

func TestRedisStore_CheckRateLimit_UnexpectedCountType(t *testing.T) {
	s, _ := newTestRedisStore(t)
	orig := rateLimitScript
	t.Cleanup(func() { rateLimitScript = orig })
	rateLimitScript = redis.NewScript(`return { "not-a-number", 1 }`)

	_, err := s.CheckRateLimit(context.Background(), "k", 3, time.Minute)
	if err == nil {
		t.Fatal("expected unexpected count type error")
	}
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

func TestRedisStore_GetRoomRegistry_UnmarshalError(t *testing.T) {
	s, mr := newTestRedisStore(t)
	mr.Set("room:BAD1", "not-json")
	_, err := s.GetRoomRegistry(context.Background(), "BAD1")
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
}

func TestRedisStore_RenewRoomRegistry_Error(t *testing.T) {
	s, mr := newTestRedisStore(t)
	mr.SetError("redis down")
	if err := s.RenewRoomRegistry(context.Background(), "R1", time.Hour); err == nil {
		t.Fatal("expected RenewRoomRegistry error")
	}
}

func TestRedisStore_RegisterRoom_Error(t *testing.T) {
	s, mr := newTestRedisStore(t)
	mr.SetError("redis down")
	if err := s.RegisterRoom(context.Background(), "R1", []byte(`{}`), time.Hour); err == nil {
		t.Fatal("expected RegisterRoom error")
	}
}

func TestRedisStore_UnregisterRoom_Error(t *testing.T) {
	s, mr := newTestRedisStore(t)
	mr.SetError("redis down")
	if err := s.UnregisterRoom(context.Background(), "R1"); err == nil {
		t.Fatal("expected UnregisterRoom error")
	}
}
