package game

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
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
	state := NewGameState("REST1")
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

	state := NewGameState("ROOM1")
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

	state := NewGameState("ROOM2")
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
	expired := now - protocol.ReconnectGraceMs - 1000
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
	state := NewGameState("LOAD1")
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
