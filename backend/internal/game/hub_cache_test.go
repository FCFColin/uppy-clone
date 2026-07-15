package game

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/store"
)

func setupHubWithMiniredis(t *testing.T, repo RoomRepository) (*Hub, *store.RedisStore) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	timeouts := config.DefaultTimeoutConfig()
	redisStore, err := store.NewRedisStore(mr.Addr(), timeouts)
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	t.Cleanup(func() { _ = redisStore.Close() })

	h := NewHub(repo, redisStore, timeouts, 0, 0)
	return h, redisStore
}

func TestHub_ListLobbiesCached_NilStore(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0)
	_, err := h.ListLobbiesCached(context.Background(), 10, "")
	if err == nil {
		t.Fatal("expected error when store is nil")
	}
}

func TestHub_ListLobbiesCached_FromStore(t *testing.T) {
	repo := newMockRoomRepository()
	ctx := context.Background()
	_ = repo.SaveLobbyState(ctx, &domain.LobbyState{
		ID: "lobby-1", Code: "ABCDE", State: "{}", UpdatedAt: time.Now().UnixMilli(),
	})

	h, _ := setupHubWithMiniredis(t, repo)
	result, err := h.ListLobbiesCached(ctx, 10, "")
	if err != nil {
		t.Fatalf("ListLobbiesCached: %v", err)
	}
	if result == nil || result.Total != 1 {
		t.Fatalf("result = %+v, want 1 lobby", result)
	}
}

func TestHub_ListLobbiesCached_CacheHit(t *testing.T) {
	repo := newMockRoomRepository()
	h, redisStore := setupHubWithMiniredis(t, repo)
	ctx := context.Background()

	cached := &domain.LobbyListResult{
		Lobbies: []domain.LobbyState{{Code: "CACHE", State: "{}"}},
		Total:   1,
	}
	data, _ := json.Marshal(cached)
	if err := redisStore.SetCachedLobbyList(ctx, 10, "", data); err != nil {
		t.Fatalf("SetCachedLobbyList: %v", err)
	}

	result, err := h.ListLobbiesCached(ctx, 10, "")
	if err != nil {
		t.Fatalf("ListLobbiesCached: %v", err)
	}
	if len(result.Lobbies) != 1 || result.Lobbies[0].Code != "CACHE" {
		t.Fatalf("cache hit result = %+v", result)
	}
}

func TestHub_CheckRoomCached_LocalRoom(t *testing.T) {
	h, _ := setupHubWithMiniredis(t, nil)
	code, err := h.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	info, err := h.CheckRoomCached(context.Background(), code)
	if err != nil {
		t.Fatalf("CheckRoomCached: %v", err)
	}
	if info == nil || info.Code != code {
		t.Fatalf("info = %+v, want code %q", info, code)
	}
}

func TestHub_CheckRoomCached_CacheHit(t *testing.T) {
	h, redisStore := setupHubWithMiniredis(t, nil)
	ctx := context.Background()
	code := "ROOM1"

	cachedInfo := RoomInfo{Code: code, Phase: "waiting", PlayerCount: 2}
	data, _ := json.Marshal(cachedInfo)
	if err := redisStore.SetCachedRoomCheck(ctx, code, data); err != nil {
		t.Fatalf("SetCachedRoomCheck: %v", err)
	}

	info, err := h.CheckRoomCached(ctx, code)
	if err != nil {
		t.Fatalf("CheckRoomCached: %v", err)
	}
	if info == nil || info.PlayerCount != 2 {
		t.Fatalf("cached info = %+v", info)
	}
}

func TestHub_InvalidateLobbyReadCaches_WithRedis(t *testing.T) {
	h, redisStore := setupHubWithMiniredis(t, nil)
	ctx := context.Background()

	data, _ := json.Marshal(&domain.LobbyListResult{Total: 1})
	_ = redisStore.SetCachedLobbyList(ctx, 10, "", data)
	roomData, _ := json.Marshal(RoomInfo{Code: "ABCDE"})
	_ = redisStore.SetCachedRoomCheck(ctx, "ABCDE", roomData)

	h.invalidateLobbyReadCaches("ABCDE")
}

func TestHub_ListLobbiesCached_NoRedis(t *testing.T) {
	repo := newMockRoomRepository()
	ctx := context.Background()
	_ = repo.SaveLobbyState(ctx, &domain.LobbyState{
		ID: "lobby-1", Code: "NORED", State: "{}", UpdatedAt: time.Now().UnixMilli(),
	})

	h := NewHub(repo, nil, config.DefaultTimeoutConfig(), 0, 0)
	result, err := h.ListLobbiesCached(ctx, 10, "")
	if err != nil {
		t.Fatalf("ListLobbiesCached: %v", err)
	}
	if len(result.Lobbies) != 1 || result.Lobbies[0].Code != "NORED" {
		t.Fatalf("result = %+v", result)
	}
}

func TestHub_CheckRoomCached_PopulatesCacheOnMiss(t *testing.T) {
	h, redisStore := setupHubWithMiniredis(t, nil)
	code, err := h.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	ctx := context.Background()
	info, err := h.CheckRoomCached(ctx, code)
	if err != nil || info == nil {
		t.Fatalf("CheckRoomCached: info=%+v err=%v", info, err)
	}

	cached, ok, err := redisStore.GetCachedRoomCheck(ctx, code)
	if err != nil || !ok {
		t.Fatalf("expected cache populated, ok=%v err=%v", ok, err)
	}
	var cachedInfo RoomInfo
	if err := json.Unmarshal(cached, &cachedInfo); err != nil {
		t.Fatalf("unmarshal cached: %v", err)
	}
	if cachedInfo.Code != code {
		t.Fatalf("cached code = %q, want %q", cachedInfo.Code, code)
	}
}

func TestHub_CheckRoomCached_InvalidCacheJSON(t *testing.T) {
	h, redisStore := setupHubWithMiniredis(t, nil)
	code, err := h.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	ctx := context.Background()
	if err := redisStore.SetCachedRoomCheck(ctx, code, []byte("{bad json")); err != nil {
		t.Fatalf("SetCachedRoomCheck: %v", err)
	}

	info, err := h.CheckRoomCached(ctx, code)
	if err != nil || info == nil || info.Code != code {
		t.Fatalf("CheckRoomCached should fall through on bad cache: info=%+v err=%v", info, err)
	}
}

func TestHub_CheckRoomCached_RedisGetError(t *testing.T) {
	h, redisStore := setupHubWithMiniredis(t, nil)
	if err := redisStore.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, err := h.CheckRoomCached(context.Background(), "ABCDE")
	if err == nil {
		t.Fatal("expected redis get error")
	}
}

func TestHub_ListLobbiesCached_CorruptCacheReloads(t *testing.T) {
	repo := newMockRoomRepository()
	ctx := context.Background()
	_ = repo.SaveLobbyState(ctx, &domain.LobbyState{
		ID: "lobby-1", Code: "FRESH", State: "{}", UpdatedAt: time.Now().UnixMilli(),
	})

	h, redisStore := setupHubWithMiniredis(t, repo)
	if err := redisStore.SetCachedLobbyList(ctx, 10, "", []byte("{invalid")); err != nil {
		t.Fatalf("SetCachedLobbyList: %v", err)
	}

	result, err := h.ListLobbiesCached(ctx, 10, "")
	if err != nil {
		t.Fatalf("ListLobbiesCached: %v", err)
	}
	if len(result.Lobbies) != 1 || result.Lobbies[0].Code != "FRESH" {
		t.Fatalf("result = %+v, want FRESH lobby from store", result)
	}
}

func TestHub_ListLobbiesCached_LoadError(t *testing.T) {
	h, _ := setupHubWithMiniredis(t, &failLoadRepo{mockRoomRepository: *newMockRoomRepository()})
	_, err := h.ListLobbiesCached(context.Background(), 10, "")
	if err == nil {
		t.Fatal("expected load error")
	}
}

type failLoadRepo struct {
	mockRoomRepository
}

func (f *failLoadRepo) LoadAllActiveLobbies(_ context.Context, _ int, _ string) (*domain.LobbyListResult, error) {
	return nil, errLoadFailed
}

var errLoadFailed = errors.New("load failed")

func TestHub_CheckRoomCached_RoomNotFound(t *testing.T) {
	h, _ := setupHubWithMiniredis(t, nil)
	info, err := h.CheckRoomCached(context.Background(), "NOPE")
	if err != nil || info != nil {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}
