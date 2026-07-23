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
	"github.com/uppy-clone/backend/internal/testutil"
)

func TestHub_registerRoomLocked_ReturnsExisting(t *testing.T) {
	h := newTestHub()
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

func TestHub_MaterializeRoom_MarksPlayersDisconnected(t *testing.T) {
	h := newTestHub()
	state := NewGameState("REST2", 42, testRNG())
	state.Players["p1"] = &domain.PlayerState{ID: "p1", Nickname: "nick1", Disconnected: false}
	state.Players["p2"] = &domain.PlayerState{ID: "p2", Nickname: "nick2", Disconnected: false}

	room := h.materializeRoom("REST2", state)

	// 恢复后所有玩家应被标记为断连，且记录断连时间戳
	for _, p := range room.state.Players {
		if !p.Disconnected {
			t.Errorf("player %s should be marked Disconnected after restore", p.ID)
		}
		if p.DisconnectedAt == nil {
			t.Errorf("player %s should have DisconnectedAt set after restore", p.ID)
		}
	}

	// usedNames 仍应正确记录
	if !room.usedNames["nick1"] || !room.usedNames["nick2"] {
		t.Fatal("expected usedNames to include nick1 and nick2")
	}

	// 断连玩家不应出现在快照中（extractSnapshotDataLocked 跳过 Disconnected 玩家）
	room.mu.RLock()
	sd := room.extractSnapshotDataLocked()
	room.mu.RUnlock()
	if len(sd.players) != 0 {
		t.Fatalf("snapshot players = %d, want 0 (disconnected players must be excluded)", len(sd.players))
	}
}

func TestHub_RestoreRooms_WithMockStore(t *testing.T) {
	h, mock, _, redisStore := setupHubWithDBAndRedis(t, 8)

	state := NewGameState("ROOM1", 42, testRNG())
	stateJSON, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	expectRestoreRoomsScan(mock, "ROOM1", string(stateJSON))

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
	h, mock, _, redisStore := setupHubWithDBAndRedis(t, 8)

	ctx := context.Background()
	foreign := []byte(`{"code":"ROOM2","instance":"other-pod","address":"10.0.0.2:8080","created_at":1}`)
	if err := redisStore.RegisterRoom(ctx, "ROOM2", foreign, time.Hour); err != nil {
		t.Fatalf("RegisterRoom: %v", err)
	}

	state := NewGameState("ROOM2", 42, testRNG())
	stateJSON, _ := json.Marshal(state)
	expectRestoreRoomsScan(mock, "ROOM2", string(stateJSON))

	if err := h.RestoreRooms(); err != nil {
		t.Fatalf("RestoreRooms: %v", err)
	}
	if h.RoomCount() != 0 {
		t.Fatalf("RoomCount = %d, want 0 for foreign-owned room", h.RoomCount())
	}
}

func TestHub_shouldLocalMaterializeRoom(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	h := NewHub(nil, redisStore, config.DefaultTimeoutConfig(), 0, 0)
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

func TestHub_loadOrMaterializeRoom_ErrorCases(t *testing.T) {
	cases := []struct {
		name      string
		code      string
		setupMock func(t *testing.T, mock pgxmock.PgxPoolIface)
	}{
		{
			name: "DeserializeError",
			code: "LOAD4",
			setupMock: func(_ *testing.T, mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states WHERE code").
					WithArgs("LOAD4").
					WillReturnRows(pgxmock.NewRows([]string{"id", "code", "state", "updated_at", "created_at"}).
						AddRow("id1", "LOAD4", "{bad", int64(1), int64(1)))
			},
		},
		{
			name: "NotFound",
			code: "MISSING",
			setupMock: func(_ *testing.T, mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states WHERE code").
					WithArgs("MISSING").
					WillReturnError(pgx.ErrNoRows)
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, mock, _ := setupHubWithDBMock(t, 8)
			c.setupMock(t, mock)
			if room := h.loadOrMaterializeRoom(c.code); room != nil {
				t.Fatal("expected nil")
			}
		})
	}
}

type paginatedRestoreRepo struct {
	*mockRoomRepository
	calls int
}

func (p *paginatedRestoreRepo) LoadAllActiveLobbies(_ context.Context, limit int, _ string) (*domain.LobbyListResult, error) {
	p.calls++
	if p.calls == 1 {
		lobbies := make([]domain.LobbyState, limit)
		for i := range lobbies {
			code := fmt.Sprintf("ROOM%03d", i+1)
			state := NewGameState(code, 42, testRNG())
			data, _ := json.Marshal(state)
			lobbies[i] = domain.LobbyState{Code: code, State: string(data)}
		}
		return &domain.LobbyListResult{Lobbies: lobbies, Total: limit + 1}, nil
	}
	state := NewGameState("ROOM101", 42, testRNG())
	data, _ := json.Marshal(state)
	return &domain.LobbyListResult{
		Lobbies: []domain.LobbyState{{Code: "ROOM101", State: string(data)}},
		Total:   limit + 1,
	}, nil
}

func TestHub_RestoreRooms_Pagination(t *testing.T) {
	repo := &paginatedRestoreRepo{mockRoomRepository: newMockRoomRepository()}
	redisStore := testutil.SetupMiniredisStore(t)
	h := NewHub(repo, redisStore, config.DefaultTimeoutConfig(), 0, 8)
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
