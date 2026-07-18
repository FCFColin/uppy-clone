package game

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
	"github.com/uppy-clone/backend/internal/store"
)

// ─── NewHub ──────────────────────────────────────────────────────────

func TestNewHub(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)
	if h == nil {
		t.Fatal("NewHub returned nil")
	}
	if h.RoomCount() != 0 {
		t.Fatalf("expected 0 rooms, got %d", h.RoomCount())
	}
}

// ─── CreateRoom ──────────────────────────────────────────────────────

func TestHub_CreateRoom(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	code, err := h.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	if len(code) != 5 {
		t.Fatalf("expected 5-char room code, got %q (len=%d)", code, len(code))
	}
	if h.RoomCount() != 1 {
		t.Fatalf("expected 1 room after CreateRoom, got %d", h.RoomCount())
	}
}

// ─── GetRoom ─────────────────────────────────────────────────────────

func TestHub_GetRoom_Found(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	code, _ := h.CreateRoom(context.Background())
	room := h.getRoom(code)
	if room == nil {
		t.Fatal("expected to find room by code")
	}
	if string(room.state.LobbyCode) != code {
		t.Fatalf("room code mismatch: got %q, want %q", string(room.state.LobbyCode), code)
	}
}

func TestHub_GetRoom_NotFound(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	room := h.getRoom("NOPE1")
	if room != nil {
		t.Fatal("expected nil for nonexistent room (no store)")
	}
}

// ─── RemoveRoom ──────────────────────────────────────────────────────

func TestHub_RemoveRoom(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	code, _ := h.CreateRoom(context.Background())
	if h.RoomCount() != 1 {
		t.Fatalf("expected 1 room, got %d", h.RoomCount())
	}

	h.RemoveRoom(context.Background(), code)
	if h.RoomCount() != 0 {
		t.Fatalf("expected 0 rooms after RemoveRoom, got %d", h.RoomCount())
	}

	// Removing nonexistent room should not panic
	h.RemoveRoom(context.Background(), "NOPE1")
}

// ─── RoomCount ───────────────────────────────────────────────────────

func TestHub_RoomCount(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	for i := 0; i < 5; i++ {
		if _, err := h.CreateRoom(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if count := h.RoomCount(); count != 5 {
		t.Fatalf("expected 5 rooms, got %d", count)
	}
}

// ─── CheckRoom ───────────────────────────────────────────────────────

func TestHub_CheckRoom_Existing(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	code, _ := h.CreateRoom(context.Background())
	info, err := h.CheckRoom(code)
	if err != nil {
		t.Fatalf("CheckRoom failed: %v", err)
	}
	if info == nil {
		t.Fatal("expected RoomInfo for existing room")
	}
	if info.Code != code {
		t.Fatalf("room code mismatch: got %q, want %q", info.Code, code)
	}
	if info.Phase != string(domain.PhaseWaiting) {
		t.Fatalf("expected phase waiting, got %q", info.Phase)
	}
}

func TestHub_CheckRoom_Nonexistent(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	info, err := h.CheckRoom("NOPE1")
	if err != nil {
		t.Fatalf("CheckRoom failed: %v", err)
	}
	if info != nil {
		t.Fatal("expected nil for nonexistent room")
	}
}

// ─── CleanupLoop ─────────────────────────────────────────────────────

func TestHub_CleanupLoop_RemovesEmptyRooms(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	// Create a room with no connections→ should be cleaned up in waiting phase
	if _, err := h.CreateRoom(context.Background()); err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	if h.RoomCount() != 1 {
		t.Fatalf("expected 1 room, got %d", h.RoomCount())
	}

	// Run one cleanup cycle
	h.cleanupOnce()

	// Room is in waiting phase with no connections→ should be removed
	if h.RoomCount() != 0 {
		t.Fatalf("expected 0 rooms after cleanup (waiting + no connections), got %d", h.RoomCount())
	}
}

func TestHub_CleanupLoop_KeepsRoomWithConnections(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	code, _ := h.CreateRoom(context.Background())
	room := h.getRoom(code)

	// Simulate a connection being present
	room.mu.Lock()
	room.connections["player1"] = &PlayerConn{PlayerID: "player1", Send: make(chan []byte, 64)}
	room.mu.Unlock()

	h.cleanupOnce()

	if h.RoomCount() != 1 {
		t.Fatalf("expected room with connections to survive cleanup, got %d rooms", h.RoomCount())
	}
}

func TestHub_CleanupLoop_KeepsPlayingRoom(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	code, _ := h.CreateRoom(context.Background())
	room := h.getRoom(code)

	// Set phase to playing (not waiting)
	room.mu.Lock()
	room.state.Phase = domain.PhasePlaying
	room.connections["player1"] = &PlayerConn{PlayerID: "player1", Send: make(chan []byte, 64)}
	room.mu.Unlock()

	h.cleanupOnce()

	if h.RoomCount() != 1 {
		t.Fatalf("expected playing room with connections to survive, got %d rooms", h.RoomCount())
	}
}

func TestHub_CleanupLoop_ContextCancellation(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		h.CleanupLoop(ctx)
		close(done)
	}()

	// Cancel after a short wait
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Success: CleanupLoop exited
	case <-time.After(2 * time.Second):
		t.Fatal("CleanupLoop did not exit after context cancellation")
	}
}

// ─── Concurrent Access ───────────────────────────────────────────────

func TestHub_ConcurrentAccess(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	var wg sync.WaitGroup

	// Writer goroutines: create rooms
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = h.CreateRoom(context.Background())
		}()
	}

	// Reader goroutines: read room count
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = h.RoomCount()
		}()
	}

	// Reader goroutines: get room
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = h.getRoom("NOPE1")
		}()
	}

	wg.Wait()

	// Should have 10 rooms
	if count := h.RoomCount(); count != 10 {
		t.Fatalf("expected 10 rooms after concurrent creation, got %d", count)
	}
}

// ─── Timeouts ────────────────────────────────────────────────────────

func TestHub_Timeouts(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	got := h.Timeouts()
	if got.PGConnectTimeout != timeouts.PGConnectTimeout {
		t.Fatalf("Timeouts mismatch: got %v, want %v", got.PGConnectTimeout, timeouts.PGConnectTimeout)
	}
}

// ─── ErrRoomCodeConflict ─────────────────────────────────────────────

func TestHub_ErrRoomCodeConflict(t *testing.T) {
	if ErrRoomCodeConflict.Error() != "room code conflict after 10 retries" {
		t.Fatalf("unexpected error message: %q", ErrRoomCodeConflict.Error())
	}
}

// ─── Concurrent CreateRoom + RemoveRoom ──────────────────────────────

func TestHub_ConcurrentCreateRemove(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	var codes []string
	for i := 0; i < 20; i++ {
		code, err := h.CreateRoom(context.Background())
		if err != nil {
			t.Fatalf("CreateRoom failed: %v", err)
		}
		codes = append(codes, code)
	}

	var wg sync.WaitGroup
	for _, code := range codes {
		wg.Add(1)
		go func(c string) {
			defer wg.Done()
			h.RemoveRoom(context.Background(), c)
		}(code)
	}
	wg.Wait()

	if count := h.RoomCount(); count != 0 {
		t.Fatalf("expected 0 rooms after removing all, got %d", count)
	}
}

func BenchmarkHub_getRoom(b *testing.B) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)
	code, _ := h.CreateRoom(context.Background())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.getRoom(code)
	}
}

func BenchmarkHub_RoomCount(b *testing.B) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)
	for i := 0; i < 100; i++ {
		_, _ = h.CreateRoom(context.Background())
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = h.RoomCount()
	}
}

func BenchmarkHub_WSConnCount(b *testing.B) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = h.WSConnCount()
	}
}

func BenchmarkHub_CanAcceptWSConnection(b *testing.B) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 1000, 50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = h.CanAcceptWSConnection()
	}
}

// ─── writePump backpressure: broadcast drops on full channel ─────────
//
// TestRoom_Broadcast_Backpressure verifies that Room.broadcast handles
// backpressure gracefully when a player's Send channel is full. The broadcast
// method uses a non-blocking select with a default case that drops the message
// and increments the ws_messages_dropped_total metric.
//
// This test ensures that a slow client (full Send buffer) does not block the
// broadcast path, which would stall the game tick for all other players.

func TestRoom_Broadcast_Backpressure(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 100, 50)
	code, _ := h.CreateRoom(context.Background())
	room := h.getRoom(code)
	room.syncOutbound = true

	// Create a PlayerConn with a buffered Send channel (capacity = WSChannelBuffer).
	pc := &PlayerConn{
		PlayerID: "player1",
		Send:     make(chan []byte, config.WSChannelBuffer),
	}
	room.mu.Lock()
	room.connections["player1"] = pc
	room.mu.Unlock()

	// Fill the Send channel to capacity.
	for i := 0; i < config.WSChannelBuffer; i++ {
		pc.Send <- []byte{protocol.MsgSnapshot}
	}

	// broadcast should return immediately (non-blocking send drops the message).
	done := make(chan struct{})
	go func() {
		room.mu.Lock()
		room.broadcast([]byte{protocol.MsgSnapshot}, "")
		room.mu.Unlock()
		close(done)
	}()

	select {
	case <-done:
		// Success: broadcast returned without blocking.
	case <-time.After(1 * time.Second):
		t.Fatal("broadcast blocked when Send channel was full")
	}

	// Verify the channel is still at capacity (message was dropped, not enqueued).
	if len(pc.Send) != config.WSChannelBuffer {
		t.Fatalf("expected Send channel to remain full (len=%d), got len=%d",
			config.WSChannelBuffer, len(pc.Send))
	}
}

// ─── writePump backpressure: broadcastCritical uses timeout ──────────
//
// TestRoom_BroadcastCritical_Backpressure verifies that broadcastCritical
// (used for phase-change messages) does not block forever when the Send channel
// is full. It uses a blocking send with a 100ms timeout per connection.

func TestRoom_BroadcastCritical_Backpressure(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 100, 50)
	code, _ := h.CreateRoom(context.Background())
	room := h.getRoom(code)
	room.syncOutbound = true

	pc := &PlayerConn{
		PlayerID: "player1",
		Send:     make(chan []byte, config.WSChannelBuffer),
	}
	room.mu.Lock()
	room.connections["player1"] = pc
	room.mu.Unlock()

	// Fill the Send channel to capacity.
	for i := 0; i < config.WSChannelBuffer; i++ {
		pc.Send <- []byte{protocol.MsgSnapshot}
	}

	// broadcastCritical should block for at most ~100ms (timeout per connection),
	// then return without enqueuing the message.
	start := time.Now()
	room.mu.Lock()
	room.broadcastCritical([]byte{protocol.MsgGameStateChange})
	room.mu.Unlock()
	elapsed := time.Since(start)
	if elapsed < 10*time.Millisecond {
		t.Fatalf("broadcastCritical returned too quickly (%v), expected to wait for timeout", elapsed)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("broadcastCritical blocked too long (%v)", elapsed)
	}

	// Verify the channel is still at capacity (message was not enqueued).
	if len(pc.Send) != config.WSChannelBuffer {
		t.Fatalf("expected Send channel to remain full (len=%d), got len=%d",
			config.WSChannelBuffer, len(pc.Send))
	}
}

// ─── Bulkhead: WS Connection Limit ──────────────────────────────────
// 企业为何需要：舱壁隔离（Bulkhead）防止单类资源耗尽拖垮整体。WebSocket 连接洪水可耗尽文件描述符和内存，
// 导致 REST API 也无法响应。连接上限是 DoS 防御的基本措施。

func TestHub_WSConnectionLimit_RejectsWhenFull(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 5, 50) // max 5 WS connections

	// Fill up to the limit
	for i := 0; i < 5; i++ {
		if !h.CanAcceptWSConnection() {
			t.Fatalf("should accept connection %d", i)
		}
		h.IncrementWSConnection()
	}

	// The 6th should be rejected
	if h.CanAcceptWSConnection() {
		t.Fatal("should reject connection when limit reached")
	}

	// Verify count
	if count := h.WSConnCount(); count != 5 {
		t.Fatalf("expected 5 connections, got %d", count)
	}
}

func TestHub_TryReserveWSConnection_Atomic(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 3, 50)
	var reserved atomic.Int32
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			if h.TryReserveWSConnection() {
				reserved.Add(1)
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	if reserved.Load() != 3 {
		t.Fatalf("reserved = %d, want 3", reserved.Load())
	}
	if h.WSConnCount() != 3 {
		t.Fatalf("WSConnCount = %d, want 3", h.WSConnCount())
	}
}

func TestHub_WSConnectionLimit_AcceptsAfterDecrement(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 3, 50)

	// Fill up
	for i := 0; i < 3; i++ {
		h.IncrementWSConnection()
	}

	if h.CanAcceptWSConnection() {
		t.Fatal("should reject when full")
	}

	// Decrement one
	h.DecrementWSConnection()

	if !h.CanAcceptWSConnection() {
		t.Fatal("should accept after decrement")
	}

	if count := h.WSConnCount(); count != 2 {
		t.Fatalf("expected 2 connections, got %d", count)
	}
}

func TestHub_WSConnectionLimit_ConcurrentIncrement(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 1000, 50)

	var wg sync.WaitGroup
	var successCount int64

	// Concurrently increment 500 times
	for i := 0; i < 500; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.IncrementWSConnection()
			atomic.AddInt64(&successCount, 1)
		}()
	}

	wg.Wait()

	if count := h.WSConnCount(); count != 500 {
		t.Fatalf("expected 500 connections, got %d", count)
	}
}

func TestHub_WSConnectionLimit_ConcurrentIncrementDecrement(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 1000, 50)

	// Start with 200 connections
	for i := 0; i < 200; i++ {
		h.IncrementWSConnection()
	}

	var wg sync.WaitGroup

	// Concurrently increment 100 and decrement 100
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			h.IncrementWSConnection()
		}()
		go func() {
			defer wg.Done()
			h.DecrementWSConnection()
		}()
	}

	wg.Wait()

	// Net should still be 200
	if count := h.WSConnCount(); count != 200 {
		t.Fatalf("expected 200 connections, got %d", count)
	}
}

func TestHub_WSConnectionLimit_DefaultValues(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0) // zero → should use defaults

	if h.MaxWSConnections() != 1000 {
		t.Fatalf("expected default max 1000, got %d", h.MaxWSConnections())
	}
	if h.MaxPlayersPerRoom() != 50 {
		t.Fatalf("expected default max players 50, got %d", h.MaxPlayersPerRoom())
	}
}

func TestHub_ErrWSConnectionLimit(t *testing.T) {
	if ErrWSConnectionLimit.Error() != "websocket connection limit reached" {
		t.Fatalf("unexpected error message: %q", ErrWSConnectionLimit.Error())
	}
}

// ─── Bulkhead: Room MaxPlayers ──────────────────────────────────────
// 企业为何需要：舱壁隔离（Bulkhead）防止单类资源耗尽拖垮整体。WebSocket 连接洪水可耗尽文件描述符和内存，
// 导致 REST API 也无法响应。连接上限是 DoS 防御的基本措施。

func TestRoom_MaxPlayers_RejectsWhenFull(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 100, 3) // max 3 players per room

	code, _ := h.CreateRoom(context.Background())
	room := h.getRoom(code)

	// Add 3 players directly
	room.mu.Lock()
	for i := 0; i < 3; i++ {
		pid := fmt.Sprintf("player%d", i)
		room.state.Players[pid] = &domain.PlayerState{
			ID:          pid,
			PlayerIndex: i,
			Nickname:    fmt.Sprintf("nick%d", i),
		}
		room.usedNames[fmt.Sprintf("nick%d", i)] = true
	}
	room.mu.Unlock()

	// 4th player should be rejected — HandleJoin with nil conn (will be closed inside)
	err := room.HandleJoin("player3", nil)
	if err != ErrRoomFull {
		t.Fatalf("expected ErrRoomFull, got %v", err)
	}
}

func TestRoom_MaxPlayers_ReconnectDoesNotCount(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 100, 2) // max 2 players per room

	code, _ := h.CreateRoom(context.Background())
	room := h.getRoom(code)

	// Add 2 players
	room.mu.Lock()
	for i := 0; i < 2; i++ {
		pid := fmt.Sprintf("player%d", i)
		room.state.Players[pid] = &domain.PlayerState{
			ID:          pid,
			PlayerIndex: i,
			Nickname:    fmt.Sprintf("nick%d", i),
		}
		room.usedNames[fmt.Sprintf("nick%d", i)] = true
	}
	room.mu.Unlock()

	// Mark player0 as disconnected
	room.mu.Lock()
	room.state.Players["player0"].Disconnected = true
	now := time.Now().UnixMilli()
	room.state.Players["player0"].DisconnectedAt = &now
	room.mu.Unlock()

	// Reconnecting player0 should succeed (not counted as new)
	err := room.HandleJoin("player0", nil)
	if err != nil {
		t.Fatalf("reconnect should succeed, got %v", err)
	}
}

func TestRoom_ErrRoomFull(t *testing.T) {
	if ErrRoomFull.Error() != "room is full" {
		t.Fatalf("unexpected error message: %q", ErrRoomFull.Error())
	}
}

func TestRoom_MaxPlayers_DefaultValue(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 100, 0) // 0 → default
	code, _ := h.CreateRoom(context.Background())
	room := h.getRoom(code)

	if room.maxPlayers != 50 {
		t.Fatalf("expected default maxPlayers 50, got %d", room.maxPlayers)
	}
}

// ─── RestoreRooms (nil store) ────────────────────────────────────────

func TestHub_RestoreRooms_NilStore(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	// With nil store, RestoreRooms should return nil
	if err := h.RestoreRooms(); err != nil {
		t.Fatalf("expected nil error with nil store, got %v", err)
	}
}

// ─── RestoreRooms (with DB) ──────────────────────────────────────────
//
// TestHub_RestoreRooms_WithDB verifies that RestoreRooms loads active rooms
// from PostgreSQL on startup. This is an integration test that requires a
// running PostgreSQL instance with the schema migrated.
//
// What the test verifies:
//  1. Rooms persisted via SaveLobbyState are loaded by RestoreRooms.
//  2. Each restored room's state (phase, players, lobby code) matches the
//     persisted state.
//  3. Restored rooms are accessible via GetRoom.
//  4. RestoreRooms is idempotent — calling it twice doesn't duplicate rooms.
//
// Skip conditions:
//   - testing.Short(): skipped in short mode (go test -short)
//   - TEST_DATABASE_URL not set: skipped when no DB is available

func TestHub_RestoreRooms_WithDB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping RestoreRooms integration test")
	}

	timeouts := config.DefaultTimeoutConfig()
	db, err := store.NewPostgresStore(dbURL, timeouts)
	if err != nil {
		t.Fatalf("failed to connect to DB: %v", err)
	}
	defer db.Close()
	gameStore := store.NewGameStore(db.Pool())

	ctx := context.Background()

	// Clean up any leftover rooms from previous test runs.
	_ = gameStore.DeleteLobbyState(ctx, "RST01")
	_ = gameStore.DeleteLobbyState(ctx, "RST02")
	defer func() {
		_ = gameStore.DeleteLobbyState(ctx, "RST01")
		_ = gameStore.DeleteLobbyState(ctx, "RST02")
	}()

	// Persist two rooms to the DB.
	state1 := NewGameState("RST01", 42, testRNG())
	state1JSON, _ := SerializeState(state1)
	if err := gameStore.SaveLobbyState(ctx, &domain.LobbyState{
		Code:      "RST01",
		State:     string(state1JSON),
		UpdatedAt: time.Now().UnixMilli(),
		CreatedAt: time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("SaveLobbyState RST01 failed: %v", err)
	}

	state2 := NewGameState("RST02", 42, testRNG())
	state2JSON, _ := SerializeState(state2)
	if err := gameStore.SaveLobbyState(ctx, &domain.LobbyState{
		Code:      "RST02",
		State:     string(state2JSON),
		UpdatedAt: time.Now().UnixMilli(),
		CreatedAt: time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("SaveLobbyState RST02 failed: %v", err)
	}

	// Create a Hub with the DB and restore rooms.
	h := NewHub(gameStore, nil, timeouts, 0, 0)
	if err := h.RestoreRooms(); err != nil {
		t.Fatalf("RestoreRooms failed: %v", err)
	}

	// Verify both rooms were restored.
	room1 := h.getRoom("RST01")
	if room1 == nil {
		t.Fatal("expected room RST01 to be restored")
	}
	if string(room1.state.LobbyCode) != "RST01" {
		t.Fatalf("restored room1 code = %q, want RST01", string(room1.state.LobbyCode))
	}

	room2 := h.getRoom("RST02")
	if room2 == nil {
		t.Fatal("expected room RST02 to be restored")
	}
	if string(room2.state.LobbyCode) != "RST02" {
		t.Fatalf("restored room2 code = %q, want RST02", string(room2.state.LobbyCode))
	}

	// Verify idempotency: calling RestoreRooms again should not duplicate rooms.
	countBefore := h.RoomCount()
	if err := h.RestoreRooms(); err != nil {
		t.Fatalf("RestoreRooms (idempotent) failed: %v", err)
	}
	if countAfter := h.RoomCount(); countAfter != countBefore {
		t.Fatalf("RestoreRooms not idempotent: before=%d, after=%d", countBefore, countAfter)
	}
}

func TestMatchRoom_NoRooms(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 2, 4)

	code, err := h.MatchRoom(context.Background())
	if err != nil {
		t.Fatalf("MatchRoom on empty hub: %v", err)
	}
	if code == "" {
		t.Fatal("MatchRoom returned empty code")
	}
}

func TestMatchRoom_FindsJoinableRoom(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 2, 4)

	code1, _ := h.CreateRoom(context.Background())

	// Join first player
	room1 := h.getRoom(code1)
	if room1 == nil {
		t.Fatal("room should exist")
	}
	room1.state.Players["p1"] = &domain.PlayerState{Nickname: "Player1", PlayerIndex: 0}

	code2, err := h.MatchRoom(context.Background())
	if err != nil {
		t.Fatalf("MatchRoom: %v", err)
	}
	// Should return the same room since it has space
	if code2 != code1 {
		t.Errorf("MatchRoom = %q, want %q (existing joinable room)", code2, code1)
	}
}

func TestMatchRoom_FullRoomsCreateNew(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 2, 1)

	code1, _ := h.CreateRoom(context.Background())
	room1 := h.getRoom(code1)

	// Fill the room
	room1.state.Players["p1"] = &domain.PlayerState{Nickname: "Player1", PlayerIndex: 0}
	room1.state.Phase = domain.PhasePlaying

	code2, err := h.MatchRoom(context.Background())
	if err != nil {
		t.Fatalf("MatchRoom: %v", err)
	}
	if code2 == code1 {
		t.Error("MatchRoom should create a new room when all rooms are full")
	}
}

func TestMatchRoom_ReturnsPlayingRoom(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 2, 4)

	code1, _ := h.CreateRoom(context.Background())
	room1 := h.getRoom(code1)
	room1.state.Phase = domain.PhasePlaying

	code2, err := h.MatchRoom(context.Background())
	if err != nil {
		t.Fatalf("MatchRoom: %v", err)
	}
	if code2 == code1 {
		t.Error("MatchRoom should not return playing rooms")
	}
}

func TestInstanceAddress(t *testing.T) {
	t.Run("uses INSTANCE_ADDR when set", func(t *testing.T) {
		if err := os.Setenv("INSTANCE_ADDR", "10.0.0.1:9000"); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Unsetenv("INSTANCE_ADDR") }()
		addr := instanceAddress()
		if addr != "10.0.0.1:9000" {
			t.Errorf("instanceAddress = %q, want %q", addr, "10.0.0.1:9000")
		}
	})

	t.Run("falls back to PORT when INSTANCE_ADDR empty", func(t *testing.T) {
		_ = os.Unsetenv("INSTANCE_ADDR")
		if err := os.Setenv("PORT", "3000"); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Unsetenv("PORT") }()
		addr := instanceAddress()
		if addr != "127.0.0.1:3000" {
			t.Errorf("instanceAddress = %q, want %q", addr, "127.0.0.1:3000")
		}
	})

	t.Run("defaults to 8080 when nothing set", func(t *testing.T) {
		_ = os.Unsetenv("INSTANCE_ADDR")
		_ = os.Unsetenv("PORT")
		addr := instanceAddress()
		if addr != "127.0.0.1:8080" {
			t.Errorf("instanceAddress = %q, want %q", addr, "127.0.0.1:8080")
		}
	})

	t.Run("returns address starting with 127.0.0.1", func(t *testing.T) {
		_ = os.Unsetenv("INSTANCE_ADDR")
		_ = os.Unsetenv("PORT")
		addr := instanceAddress()
		if !strings.HasPrefix(addr, "127.0.0.1:") {
			t.Errorf("instanceAddress = %q, want 127.0.0.1:… prefix", addr)
		}
	})
}

func TestHub_CreateRoom_CodeConflict(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)
	h.rooms["CONFL"] = NewRoom("CONFL", h, nil, timeouts, 0)

	restore := h.SetGenerateRoomCodeHook(func() string { return "CONFL" })
	defer restore()

	_, err := h.CreateRoom(context.Background())
	if err != ErrRoomCodeConflict {
		t.Fatalf("CreateRoom error = %v, want ErrRoomCodeConflict", err)
	}
}

func TestHub_CreateRoom_WithRedis(t *testing.T) {
	h, redisStore := setupHubWithMiniredis(t, nil)

	code, err := h.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if h.getRoom(code) == nil {
		t.Fatal("room should exist after CreateRoom")
	}

	ctx := context.Background()
	info, err := redisStore.GetRoomRegistry(ctx, code)
	if err != nil {
		t.Fatalf("GetRoomRegistry: %v", err)
	}
	if info == nil || info.Code != code {
		t.Fatalf("registry info = %+v, want code %q", info, code)
	}
}

func TestHub_RemoveRoom_DeletesFromStore(t *testing.T) {
	repo := newMockRoomRepository()
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(repo, nil, timeouts, 0, 0)

	code, err := h.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	h.RemoveRoom(context.Background(), code)

	if repo.deleteCount != 1 {
		t.Fatalf("deleteCount = %d, want 1", repo.deleteCount)
	}
	if h.RoomCount() != 0 {
		t.Fatalf("RoomCount = %d, want 0", h.RoomCount())
	}
}

func TestHub_RemoveRoom_WithRedis(t *testing.T) {
	h, redisStore := setupHubWithMiniredis(t, newMockRoomRepository())
	code, err := h.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	h.RemoveRoom(context.Background(), code)
	if h.RoomCount() != 0 {
		t.Fatalf("RoomCount = %d, want 0", h.RoomCount())
	}

	info, _ := redisStore.GetRoomRegistry(context.Background(), code)
	if info != nil {
		t.Fatal("room should be unregistered from redis after RemoveRoom")
	}
}

func TestHub_CleanupLoop_RemovesAllDisconnectedExpired(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	code, _ := h.CreateRoom(context.Background())
	room := h.getRoom(code)
	disconnectedAt := time.Now().UnixMilli() - domain.ReconnectGraceMs - 1000
	room.mu.Lock()
	room.state.Players["p1"] = &domain.PlayerState{
		ID:             "p1",
		Nickname:       "gone",
		Disconnected:   true,
		DisconnectedAt: &disconnectedAt,
	}
	room.mu.Unlock()

	h.cleanupOnce()
	if h.RoomCount() != 0 {
		t.Fatalf("expected expired disconnected room removed, got %d rooms", h.RoomCount())
	}
}

func TestHub_CleanupLoop_RemovesZeroPlayerRoom(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	code, _ := h.CreateRoom(context.Background())
	room := h.getRoom(code)
	room.mu.Lock()
	room.state.Phase = domain.PhasePlaying
	room.mu.Unlock()

	h.cleanupOnce()
	if h.RoomCount() != 0 {
		t.Fatalf("expected zero-player room removed, got %d rooms", h.RoomCount())
	}
}

func TestHub_CleanupLoop_KeepsDisconnectedInGrace(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0)

	code, _ := h.CreateRoom(context.Background())
	room := h.getRoom(code)
	disconnectedAt := time.Now().UnixMilli() - 1000
	room.mu.Lock()
	room.state.Phase = domain.PhasePlaying
	room.state.Players["p1"] = &domain.PlayerState{
		ID:             "p1",
		Nickname:       "grace",
		Disconnected:   true,
		DisconnectedAt: &disconnectedAt,
	}
	room.mu.Unlock()

	h.cleanupOnce()
	if h.RoomCount() != 1 {
		t.Fatalf("room in grace period should survive cleanup, got %d rooms", h.RoomCount())
	}
}

func TestMatchRoom_SkipsRoomWithFullConnections(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 2, 2)

	code1, _ := h.CreateRoom(context.Background())
	room1 := h.getRoom(code1)
	room1.mu.Lock()
	room1.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 4)}
	room1.connections["p2"] = &PlayerConn{PlayerID: "p2", Send: make(chan []byte, 4)}
	room1.mu.Unlock()

	code2, err := h.MatchRoom(context.Background())
	if err != nil {
		t.Fatalf("MatchRoom: %v", err)
	}
	if code2 == code1 {
		t.Fatal("MatchRoom should skip room at connection capacity")
	}
}

func TestRoomJoinable_RequiresWaitingPhase(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	room := NewRoom("TEST1", nil, nil, timeouts, 4)
	room.state.Phase = domain.PhasePlaying
	if roomJoinable(room, 4) {
		t.Fatal("playing room should not be joinable")
	}
}

func TestHub_removeRooms_EmptyBatch(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0)
	h.removeRooms(nil, removeRoomOptions{pgDelete: true})
	h.removeRooms([]string{"MISSING"}, removeRoomOptions{pgDelete: true})
	if h.RoomCount() != 0 {
		t.Fatalf("RoomCount = %d, want 0", h.RoomCount())
	}
}

func TestInstanceAddress_DefaultsToLocalhostPort(t *testing.T) {
	_ = os.Unsetenv("INSTANCE_ADDR")
	_ = os.Unsetenv("PORT")
	if got := instanceAddress(); got != "127.0.0.1:8080" {
		t.Fatalf("instanceAddress() = %q", got)
	}
}

func TestHub_CleanupLoop_RunsCleanupOnce(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0)
	code, _ := h.CreateRoom(context.Background())
	room := h.getRoom(code)
	room.mu.Lock()
	room.state.Phase = domain.PhaseWaiting
	room.connections = make(map[string]*PlayerConn)
	room.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		h.CleanupLoop(ctx)
		close(done)
	}()
	h.cleanupOnce()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("CleanupLoop did not exit")
	}
	if h.getRoom(code) != nil {
		t.Fatal("empty waiting room should be cleaned up")
	}
}

// --- coverage gap 补充用例 ---

func TestHub_CleanupOnce_SkipsMissingRoom(_ *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0)
	h.mu.Lock()
	h.rooms["GHOST"] = NewRoom("GHOST", h, nil, config.DefaultTimeoutConfig(), 4)
	delete(h.rooms, "GHOST")
	h.mu.Unlock()
	h.cleanupOnce()
}

func TestDefaultInstanceID_UnknownOnEmptyHostname(t *testing.T) {
	t.Setenv("INSTANCE_ID", "")
	got := defaultInstanceID(func() (string, error) { return "", errors.New("no hostname") })
	if got != unknownPlayerID {
		t.Fatalf("defaultInstanceID = %q, want unknown", got)
	}
}

func TestHub_cleanupOnce_KeepsPlayingRoom(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0)
	code, _ := h.CreateRoom(context.Background())
	room := h.getRoom(code)
	addConnectedPlayer(room, "p1")
	room.mu.Lock()
	room.state.Phase = domain.PhasePlaying
	room.mu.Unlock()
	h.cleanupOnce()
	if h.getRoom(code) == nil {
		t.Fatal("playing room with connections should not be cleaned")
	}
}
func TestHub_cleanupOnce_KeepsDisconnectedWithoutTimestamp(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0)
	code, _ := h.CreateRoom(context.Background())
	room := h.getRoom(code)
	room.mu.Lock()
	room.state.Phase = domain.PhasePlaying
	room.state.Players["p1"] = &domain.PlayerState{ID: "p1", Disconnected: true, DisconnectedAt: nil}
	room.connections = make(map[string]*PlayerConn)
	room.mu.Unlock()
	h.cleanupOnce()
	if h.getRoom(code) == nil {
		t.Fatal("room with disconnected player without timestamp should not be cleaned")
	}
}

func TestHub_CreateRoom_AllConflicts(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0)
	restore := h.SetGenerateRoomCodeHook(func() string { return "SAME1" })
	defer restore()
	h.mu.Lock()
	h.rooms["SAME1"] = NewRoom("SAME1", h, nil, config.DefaultTimeoutConfig(), 4)
	h.mu.Unlock()
	_, err := h.CreateRoom(context.Background())
	if err != ErrRoomCodeConflict {
		t.Fatalf("err = %v, want ErrRoomCodeConflict", err)
	}
}

func TestHub_MatchRoom_SkipsNonJoinable(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0)
	code, _ := h.CreateRoom(context.Background())
	room := h.getRoom(code)
	room.mu.Lock()
	room.state.Phase = domain.PhasePlaying
	room.mu.Unlock()
	code2, err := h.MatchRoom(context.Background())
	if err != nil || code2 == code {
		t.Fatalf("MatchRoom = %q err=%v", code2, err)
	}
}

func TestHub_CreateRoom_InvalidCodeLogged(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0)
	restore := h.SetGenerateRoomCodeHook(func() string { return "AB0DE" })
	defer restore()
	code, err := h.CreateRoom(context.Background())
	if err == nil {
		t.Fatalf("expected error for invalid room code, got code=%q", code)
	}
}
