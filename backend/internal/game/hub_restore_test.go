package game

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/testutil"
)

func TestHub_registerRoomLocked_ReturnsExisting(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	existing := NewRoom("DUP", h, nil, config.DefaultTimeoutConfig(), 0)
	other := NewRoom("DUP", h, nil, config.DefaultTimeoutConfig(), 0)
	h.mu.Lock()
	got := h.registerRoomLocked("DUP", existing)
	replaced := h.registerRoomLocked("DUP", other)
	h.mu.Unlock()
	if got != existing || replaced != existing {
		t.Fatal("registerRoomLocked should return first registered room")
	}
}

func TestHub_MaterializeRoom(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	state := NewGameState("REST1", testRNG())
	state.Players["p1"] = &domain.PlayerState{ID: "p1", Nickname: "nick1"}
	room := h.materializeRoom("REST1", state)
	if room == nil || room.state.LobbyCode != "REST1" {
		t.Fatalf("room = %+v", room)
	}
	if !room.usedNames["nick1"] {
		t.Fatal("expected usedNames to include nick1")
	}
}

func TestHub_DeserializeAndMaterialize_Invalid(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	if _, err := h.deserializeAndMaterialize("X", []byte(`{invalid`)); err == nil {
		t.Fatal("expected deserialize error")
	}
}

func TestHub_RestoreRooms_WithMockStore(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

	db := store.NewPostgresStoreWithPool(mock)
	redisStore := testutil.SetupMiniredisStore(t)
	bc := newMockBroadcaster()
	h := NewHub(db, redisStore, config.DefaultTimeoutConfig(), 0, 8, bc)

	state := NewGameState("ROOM1", testRNG())
	stateJSON, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM lobby_states").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states").
		WithArgs(101).
		WillReturnRows(pgxmock.NewRows([]string{"id", "code", "state", "updated_at", "created_at"}).
			AddRow("id1", "ROOM1", string(stateJSON), int64(100), int64(50)))

	if err := h.RestoreRooms(); err != nil {
		t.Fatalf("RestoreRooms: %v", err)
	}
	if h.RoomCount() != 1 {
		t.Fatalf("RoomCount = %d", h.RoomCount())
	}

	ctx := context.Background()
	info, err := redisStore.GetRoomRegistry(ctx, "ROOM1")
	if err != nil {
		t.Fatalf("GetRoomRegistry: %v", err)
	}
	if info == nil || info.Instance != h.instanceID {
		t.Fatalf("registry = %+v, instanceID = %q", info, h.instanceID)
	}
}

func TestHub_RestoreRooms_SkipsForeignOwner(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

	db := store.NewPostgresStoreWithPool(mock)
	redisStore := testutil.SetupMiniredisStore(t)
	h := NewHub(db, redisStore, config.DefaultTimeoutConfig(), 0, 8, nil)

	ctx := context.Background()
	foreign := []byte(`{"code":"ROOM2","instance":"other-pod","address":"10.0.0.2:8080","created_at":1}`)
	if err := redisStore.RegisterRoom(ctx, "ROOM2", foreign, time.Hour); err != nil {
		t.Fatalf("RegisterRoom: %v", err)
	}

	state := NewGameState("ROOM2", testRNG())
	stateJSON, _ := json.Marshal(state)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM lobby_states").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states").
		WithArgs(101).
		WillReturnRows(pgxmock.NewRows([]string{"id", "code", "state", "updated_at", "created_at"}).
			AddRow("id2", "ROOM2", string(stateJSON), int64(100), int64(50)))

	if err := h.RestoreRooms(); err != nil {
		t.Fatalf("RestoreRooms: %v", err)
	}
	if h.RoomCount() != 0 {
		t.Fatalf("RoomCount = %d, want 0 for foreign-owned room", h.RoomCount())
	}
}

func TestHub_shouldLocalMaterializeRoom(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	h := NewHub(nil, redisStore, config.DefaultTimeoutConfig(), 0, 0, nil)
	ctx := context.Background()

	if !h.shouldLocalMaterializeRoom(ctx, "MISS1") {
		t.Fatal("expected miss registry to allow local materialize")
	}

	foreign := []byte(`{"code":"OWN1","instance":"other","address":"x","created_at":1}`)
	_ = redisStore.RegisterRoom(ctx, "OWN1", foreign, time.Hour)
	if h.shouldLocalMaterializeRoom(ctx, "OWN1") {
		t.Fatal("expected foreign owner to skip local materialize")
	}

	own := []byte(`{"code":"OWN2","instance":"` + h.instanceID + `","address":"x","created_at":1}`)
	_ = redisStore.RegisterRoom(ctx, "OWN2", own, time.Hour)
	if !h.shouldLocalMaterializeRoom(ctx, "OWN2") {
		t.Fatal("expected own instance to allow local materialize")
	}
}

func TestHub_subscribeRoom(t *testing.T) {
	bc := newMockBroadcaster()
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, bc)
	h.subscribeRoom("CODE1")
	h.subscribeRoom("CODE1")
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.subscriptions) != 1 {
		t.Fatalf("subscriptions = %d", len(h.subscriptions))
	}
}

func TestHub_unregisterRoomFromRedis(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	h := NewHub(nil, redisStore, config.DefaultTimeoutConfig(), 0, 0, nil)
	h.registerRoomInRedis("CODE1")
	h.unregisterRoomFromRedis("CODE1")
	h.unregisterRoomFromRedis("CODE1")
}

func TestHub_unsubscribeRoomLocked(t *testing.T) {
	bc := newMockBroadcaster()
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, bc)
	h.subscribeRoom("CODE1")
	h.mu.Lock()
	h.unsubscribeRoomLocked("CODE1")
	h.mu.Unlock()
	h.mu.Lock()
	if len(h.subscriptions) != 0 {
		t.Fatal("expected subscription removed")
	}
	h.mu.Unlock()
}

func TestHub_unsubscribeRoomLocked_NoSubscription(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	h.mu.Lock()
	h.unsubscribeRoomLocked("NONE")
	h.mu.Unlock()
}

func TestHub_subscribeRoom_RemoteBroadcast(t *testing.T) {
	bc := newMockBroadcaster()
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, bc)
	h.instanceID = "inst-A"

	room := NewRoom("ROOM1", h, nil, config.DefaultTimeoutConfig(), 0)
	room.syncOutbound = true
	h.mu.Lock()
	h.rooms["ROOM1"] = room
	h.mu.Unlock()

	ch := make(chan []byte, 64)
	room.mu.Lock()
	room.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch}
	room.mu.Unlock()

	h.subscribeRoom("ROOM1")

	if err := bc.Publish(context.Background(), "ROOM1", BroadcastMessage{
		ExcludeInstance: "inst-B",
		Payload:         []byte("remote"),
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case got := <-ch:
		if string(got) != "remote" {
			t.Fatalf("payload = %q, want remote", string(got))
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for remote broadcast")
	}
}

func TestAllPlayersDisconnectedExpired(t *testing.T) {
	now := time.Now().UnixMilli()
	expired := now - domain.ReconnectGraceMs - 1000
	players := map[string]*domain.PlayerState{
		"p1": {Disconnected: true, DisconnectedAt: &expired},
	}
	if !allPlayersDisconnectedExpired(players, now) {
		t.Fatal("expected all expired")
	}
	players["p2"] = &domain.PlayerState{Disconnected: false}
	if allPlayersDisconnectedExpired(players, now) {
		t.Fatal("expected false when one connected")
	}
}

func TestRoom_handleCountdownEnd(t *testing.T) {
	r := NewRoom("CD1", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.state.Phase = domain.PhaseCountdown
	r.state.SessionID = "sess-1"
	r.state.StartedAt = time.Now().UnixMilli()
	r.handleCountdownEnd()
	if r.state.Phase != domain.PhasePlaying {
		t.Fatalf("phase = %s", r.state.Phase)
	}
	if r.state.TickCount != 1 {
		t.Fatalf("TickCount = %d", r.state.TickCount)
	}
}

func TestRoom_handleAutoRestart_NoPlayers(t *testing.T) {
	r := NewRoom("AR1", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.state.Phase = domain.PhaseEnded
	r.handleAutoRestart()
	if r.state.Phase != domain.PhaseWaiting {
		t.Fatalf("phase = %s", r.state.Phase)
	}
}

func TestRoom_handleAutoRestart_WithVotes(t *testing.T) {
	r := NewRoom("AR2", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.state.Phase = domain.PhaseEnded
	addConnectedPlayer(r, "p1")
	r.state.RestartVotes = map[string]bool{"p1": true}
	r.handleAutoRestart()
	if r.state.Phase != domain.PhaseEnded {
		t.Fatalf("phase = %s, expected deferred restart", r.state.Phase)
	}
}

func TestHub_loadOrMaterializeRoom(t *testing.T) {
	state := NewGameState("LOAD1", testRNG())
	stateJSON, _ := json.Marshal(state)

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
	mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states WHERE code").
		WithArgs("LOAD1").
		WillReturnRows(pgxmock.NewRows([]string{"id", "code", "state", "updated_at", "created_at"}).
			AddRow("id1", "LOAD1", string(stateJSON), int64(1), int64(1)))

	bc := newMockBroadcaster()
	h := NewHub(db, nil, config.DefaultTimeoutConfig(), 0, 0, bc)
	room := h.loadOrMaterializeRoom("LOAD1")
	if room == nil {
		t.Fatal("expected room")
	}
}

func TestHub_shouldLocalMaterializeRoom_RedisError(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	h := NewHub(nil, redisStore, config.DefaultTimeoutConfig(), 0, 0, nil)
	_ = redisStore.Close()
	if h.shouldLocalMaterializeRoom(context.Background(), "ERR1") {
		t.Fatal("redis error should not allow local materialize")
	}
}

func TestHub_subscribeRoom_NilBroadcaster(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	h.subscribeRoom("CODE1")
}

func TestHub_subscribeRoom_SubscribeError(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, &subscribeErrBroadcaster{})
	h.subscribeRoom("CODE1")
}

func TestHub_loadOrMaterializeRoom_NilStore(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	if room := h.loadOrMaterializeRoom("X"); room != nil {
		t.Fatal("expected nil without store")
	}
}

func TestHub_RestoreRooms_LoadError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM lobby_states").
		WillReturnError(context.Canceled)

	h := NewHub(db, nil, config.DefaultTimeoutConfig(), 0, 8, nil)
	if err := h.RestoreRooms(); err == nil {
		t.Fatal("expected load error")
	}
}

func TestHub_RestoreRooms_SkipsExistingRoom(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)

	state := NewGameState("EXIST", testRNG())
	stateJSON, _ := json.Marshal(state)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM lobby_states").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states").
		WithArgs(101).
		WillReturnRows(pgxmock.NewRows([]string{"id", "code", "state", "updated_at", "created_at"}).
			AddRow("id1", "EXIST", string(stateJSON), int64(100), int64(50)))

	h := NewHub(db, nil, config.DefaultTimeoutConfig(), 0, 8, nil)
	h.mu.Lock()
	h.rooms["EXIST"] = NewRoom("EXIST", h, db, config.DefaultTimeoutConfig(), 8)
	h.mu.Unlock()

	if err := h.RestoreRooms(); err != nil {
		t.Fatalf("RestoreRooms: %v", err)
	}
}

func TestHub_RestoreRooms_DeserializeError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM lobby_states").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states").
		WithArgs(101).
		WillReturnRows(pgxmock.NewRows([]string{"id", "code", "state", "updated_at", "created_at"}).
			AddRow("id1", "BAD1", "{invalid", int64(100), int64(50)))

	h := NewHub(db, nil, config.DefaultTimeoutConfig(), 0, 8, nil)
	if err := h.RestoreRooms(); err != nil {
		t.Fatalf("RestoreRooms should continue on deserialize error: %v", err)
	}
	if h.RoomCount() != 0 {
		t.Fatalf("RoomCount = %d", h.RoomCount())
	}
}

func TestHub_loadOrMaterializeRoom_ForeignOwner(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	foreign := []byte(`{"code":"LOAD2","instance":"other","address":"x","created_at":1}`)
	_ = redisStore.RegisterRoom(context.Background(), "LOAD2", foreign, time.Hour)
	h := NewHub(nil, redisStore, config.DefaultTimeoutConfig(), 0, 8, nil)
	if room := h.loadOrMaterializeRoom("LOAD2"); room != nil {
		t.Fatal("expected nil for foreign owner")
	}
}

func TestHub_loadOrMaterializeRoom_ForeignOwnerWithStore(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)

	redisStore := testutil.SetupMiniredisStore(t)
	foreign := []byte(`{"code":"LOAD5","instance":"other","address":"x","created_at":1}`)
	_ = redisStore.RegisterRoom(context.Background(), "LOAD5", foreign, time.Hour)

	h := NewHub(db, redisStore, config.DefaultTimeoutConfig(), 0, 8, nil)
	if room := h.loadOrMaterializeRoom("LOAD5"); room != nil {
		t.Fatal("expected nil when foreign owner skips materialization")
	}
}

func TestHub_loadOrMaterializeRoom_LoadError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
	mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states WHERE code").
		WithArgs("LOAD3").
		WillReturnError(context.Canceled)

	h := NewHub(db, nil, config.DefaultTimeoutConfig(), 0, 8, nil)
	if room := h.loadOrMaterializeRoom("LOAD3"); room != nil {
		t.Fatal("expected nil on load error")
	}
}

func TestHub_loadOrMaterializeRoom_DeserializeError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
	mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states WHERE code").
		WithArgs("LOAD4").
		WillReturnRows(pgxmock.NewRows([]string{"id", "code", "state", "updated_at", "created_at"}).
			AddRow("id1", "LOAD4", "{bad", int64(1), int64(1)))

	h := NewHub(db, nil, config.DefaultTimeoutConfig(), 0, 8, nil)
	if room := h.loadOrMaterializeRoom("LOAD4"); room != nil {
		t.Fatal("expected nil on deserialize error")
	}
}

func TestHub_loadOrMaterializeRoom_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
	mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states WHERE code").
		WithArgs("MISSING").
		WillReturnError(pgx.ErrNoRows)

	h := NewHub(db, nil, config.DefaultTimeoutConfig(), 0, 8, nil)
	if room := h.loadOrMaterializeRoom("MISSING"); room != nil {
		t.Fatal("expected nil when lobby not found")
	}
}

type paginatedRestoreRepo struct {
	*mockRoomRepository
	calls int
}

func (p *paginatedRestoreRepo) LoadAllActiveLobbies(_ context.Context, limit int, cursor string) (*domain.LobbyListResult, error) {
	p.calls++
	if p.calls == 1 {
		lobbies := make([]domain.LobbyState, limit)
		for i := range lobbies {
			code := fmt.Sprintf("ROOM%03d", i+1)
			state := NewGameState(code, testRNG())
			data, _ := json.Marshal(state)
			lobbies[i] = domain.LobbyState{Code: code, State: string(data)}
		}
		return &domain.LobbyListResult{Lobbies: lobbies, Total: limit + 1}, nil
	}
	state := NewGameState("ROOM101", testRNG())
	data, _ := json.Marshal(state)
	return &domain.LobbyListResult{
		Lobbies: []domain.LobbyState{{Code: "ROOM101", State: string(data)}},
		Total:   limit + 1,
	}, nil
}

func TestHub_RestoreRooms_Pagination(t *testing.T) {
	repo := &paginatedRestoreRepo{mockRoomRepository: newMockRoomRepository()}
	redisStore := testutil.SetupMiniredisStore(t)
	h := NewHub(repo, redisStore, config.DefaultTimeoutConfig(), 0, 8, newMockBroadcaster())
	if err := h.RestoreRooms(); err != nil {
		t.Fatalf("RestoreRooms: %v", err)
	}
	if h.RoomCount() != 101 {
		t.Fatalf("RoomCount = %d, want 101", h.RoomCount())
	}
	if repo.calls < 2 {
		t.Fatalf("LoadAllActiveLobbies calls = %d, want >= 2", repo.calls)
	}
}

type emptyRestoreRepo struct {
	*mockRoomRepository
}

func (e *emptyRestoreRepo) LoadAllActiveLobbies(context.Context, int, string) (*domain.LobbyListResult, error) {
	return &domain.LobbyListResult{Lobbies: []domain.LobbyState{}}, nil
}

func TestHub_RestoreRooms_EmptyPage(t *testing.T) {
	h := NewHub(&emptyRestoreRepo{mockRoomRepository: newMockRoomRepository()}, nil, config.DefaultTimeoutConfig(), 0, 8, nil)
	if err := h.RestoreRooms(); err != nil {
		t.Fatalf("RestoreRooms: %v", err)
	}
	if h.RoomCount() != 0 {
		t.Fatalf("RoomCount = %d, want 0", h.RoomCount())
	}
}

func TestHub_loadOrMaterializeRoom_ReturnsExisting(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
	state := NewGameState("EXIST1", testRNG())
	stateJSON, _ := json.Marshal(state)

	mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states WHERE code").
		WithArgs("EXIST1").
		WillReturnRows(pgxmock.NewRows([]string{"id", "code", "state", "updated_at", "created_at"}).
			AddRow("id1", "EXIST1", string(stateJSON), int64(1), int64(1)))

	h := NewHub(db, nil, config.DefaultTimeoutConfig(), 0, 8, nil)
	existing := NewRoom("EXIST1", h, db, config.DefaultTimeoutConfig(), 0)
	h.mu.Lock()
	h.rooms["EXIST1"] = existing
	h.mu.Unlock()

	if room := h.loadOrMaterializeRoom("EXIST1"); room != existing {
		t.Fatal("expected existing room pointer")
	}
}
