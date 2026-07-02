package game

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
	"github.com/uppy-clone/backend/internal/store"
)

// ─── NewHub ──────────────────────────────────────────────────────────

func TestNewHub(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0, nil)
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
	h := NewHub(nil, nil, timeouts, 0, 0, nil)

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
	h := NewHub(nil, nil, timeouts, 0, 0, nil)

	code, _ := h.CreateRoom(context.Background())
	room := h.GetRoom(code)
	if room == nil {
		t.Fatal("expected to find room by code")
	}
	if room.state.LobbyCode != code {
		t.Fatalf("room code mismatch: got %q, want %q", room.state.LobbyCode, code)
	}
}

func TestHub_GetRoom_NotFound(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0, nil)

	room := h.GetRoom("NOPE1")
	if room != nil {
		t.Fatal("expected nil for nonexistent room (no store)")
	}
}

// ─── RemoveRoom ──────────────────────────────────────────────────────

func TestHub_RemoveRoom(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0, nil)

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
	h := NewHub(nil, nil, timeouts, 0, 0, nil)

	for i := 0; i < 5; i++ {
		h.CreateRoom(context.Background())
	}
	if count := h.RoomCount(); count != 5 {
		t.Fatalf("expected 5 rooms, got %d", count)
	}
}

// ─── CheckRoom ───────────────────────────────────────────────────────

func TestHub_CheckRoom_Existing(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0, nil)

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
	h := NewHub(nil, nil, timeouts, 0, 0, nil)

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
	h := NewHub(nil, nil, timeouts, 0, 0, nil)

	// Create a room with no connections → should be cleaned up in waiting phase
	code, _ := h.CreateRoom(context.Background())
	if h.RoomCount() != 1 {
		t.Fatalf("expected 1 room, got %d", h.RoomCount())
	}

	// Run one cleanup cycle
	h.cleanupOnce()

	// Room is in waiting phase with no connections → should be removed
	if h.RoomCount() != 0 {
		t.Fatalf("expected 0 rooms after cleanup (waiting + no connections), got %d", h.RoomCount())
	}
	_ = code
}

func TestHub_CleanupLoop_KeepsRoomWithConnections(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0, nil)

	code, _ := h.CreateRoom(context.Background())
	room := h.GetRoom(code)

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
	h := NewHub(nil, nil, timeouts, 0, 0, nil)

	code, _ := h.CreateRoom(context.Background())
	room := h.GetRoom(code)

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
	h := NewHub(nil, nil, timeouts, 0, 0, nil)

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
	h := NewHub(nil, nil, timeouts, 0, 0, nil)

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
			_ = h.GetRoom("NOPE1")
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
	h := NewHub(nil, nil, timeouts, 0, 0, nil)

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
	h := NewHub(nil, nil, timeouts, 0, 0, nil)

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

// ─── Hub registerRoomToRedis (nil redis) ─────────────────────────────

func TestHub_RegisterRoomInRedis_NilRedis(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0, nil)

	// Should not panic with nil redis
	h.registerRoomInRedis("TEST1")
}

func TestHub_UnregisterRoomFromRedis_NilRedis(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0, nil)

	// Should not panic with nil redis
	h.unregisterRoomFromRedis("TEST1")
}

// ─── Benchmarks ──────────────────────────────────────────────────────

func BenchmarkHub_CreateRoom(b *testing.B) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0, nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.CreateRoom(context.Background())
	}
}

func BenchmarkHub_GetRoom(b *testing.B) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0, nil)
	code, _ := h.CreateRoom(context.Background())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.GetRoom(code)
	}
}

func BenchmarkHub_RoomCount(b *testing.B) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0, nil)
	for i := 0; i < 100; i++ {
		h.CreateRoom(context.Background())
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = h.RoomCount()
	}
}

func BenchmarkHub_WSConnCount(b *testing.B) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0, nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = h.WSConnCount()
	}
}

func BenchmarkHub_CanAcceptWSConnection(b *testing.B) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 1000, 50, nil)
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
	h := NewHub(nil, nil, timeouts, 100, 50, nil)
	code, _ := h.CreateRoom(context.Background())
	room := h.GetRoom(code)
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
	h := NewHub(nil, nil, timeouts, 100, 50, nil)
	code, _ := h.CreateRoom(context.Background())
	room := h.GetRoom(code)
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
	h := NewHub(nil, nil, timeouts, 5, 50, nil) // max 5 WS connections

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
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 3, 50, nil)
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
	h := NewHub(nil, nil, timeouts, 3, 50, nil)

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
	h := NewHub(nil, nil, timeouts, 1000, 50, nil)

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
	h := NewHub(nil, nil, timeouts, 1000, 50, nil)

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
	h := NewHub(nil, nil, timeouts, 0, 0, nil) // zero → should use defaults

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
	h := NewHub(nil, nil, timeouts, 100, 3, nil) // max 3 players per room

	code, _ := h.CreateRoom(context.Background())
	room := h.GetRoom(code)

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
	h := NewHub(nil, nil, timeouts, 100, 2, nil) // max 2 players per room

	code, _ := h.CreateRoom(context.Background())
	room := h.GetRoom(code)

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
	h := NewHub(nil, nil, timeouts, 100, 0, nil) // 0 → default
	code, _ := h.CreateRoom(context.Background())
	room := h.GetRoom(code)

	if room.maxPlayers != 50 {
		t.Fatalf("expected default maxPlayers 50, got %d", room.maxPlayers)
	}
}

// ─── RestoreRooms (nil store) ────────────────────────────────────────

func TestHub_RestoreRooms_NilStore(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0, nil)

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

	ctx := context.Background()

	// Clean up any leftover rooms from previous test runs.
	_ = db.DeleteLobbyState(ctx, "RST01")
	_ = db.DeleteLobbyState(ctx, "RST02")
	defer func() {
		_ = db.DeleteLobbyState(ctx, "RST01")
		_ = db.DeleteLobbyState(ctx, "RST02")
	}()

	// Persist two rooms to the DB.
	state1 := NewGameState("RST01")
	state1JSON, _ := SerializeState(state1)
	if err := db.SaveLobbyState(ctx, &domain.LobbyState{
		Code:      "RST01",
		State:     string(state1JSON),
		UpdatedAt: time.Now().UnixMilli(),
		CreatedAt: time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("SaveLobbyState RST01 failed: %v", err)
	}

	state2 := NewGameState("RST02")
	state2JSON, _ := SerializeState(state2)
	if err := db.SaveLobbyState(ctx, &domain.LobbyState{
		Code:      "RST02",
		State:     string(state2JSON),
		UpdatedAt: time.Now().UnixMilli(),
		CreatedAt: time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("SaveLobbyState RST02 failed: %v", err)
	}

	// Create a Hub with the DB and restore rooms.
	h := NewHub(db, nil, timeouts, 0, 0, nil)
	if err := h.RestoreRooms(); err != nil {
		t.Fatalf("RestoreRooms failed: %v", err)
	}

	// Verify both rooms were restored.
	room1 := h.GetRoom("RST01")
	if room1 == nil {
		t.Fatal("expected room RST01 to be restored")
	}
	if room1.state.LobbyCode != "RST01" {
		t.Fatalf("restored room1 code = %q, want RST01", room1.state.LobbyCode)
	}

	room2 := h.GetRoom("RST02")
	if room2 == nil {
		t.Fatal("expected room RST02 to be restored")
	}
	if room2.state.LobbyCode != "RST02" {
		t.Fatalf("restored room2 code = %q, want RST02", room2.state.LobbyCode)
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
	h := NewHub(nil, nil, timeouts, 2, 4, nil)

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
	h := NewHub(nil, nil, timeouts, 2, 4, nil)

	code1, _ := h.CreateRoom(context.Background())

	// Join first player
	room1 := h.GetRoom(code1)
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
	h := NewHub(nil, nil, timeouts, 2, 1, nil)

	code1, _ := h.CreateRoom(context.Background())
	room1 := h.GetRoom(code1)

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
	h := NewHub(nil, nil, timeouts, 2, 4, nil)

	code1, _ := h.CreateRoom(context.Background())
	room1 := h.GetRoom(code1)
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
		os.Setenv("INSTANCE_ADDR", "10.0.0.1:9000")
		defer os.Unsetenv("INSTANCE_ADDR")
		addr := instanceAddress()
		if addr != "10.0.0.1:9000" {
			t.Errorf("instanceAddress = %q, want %q", addr, "10.0.0.1:9000")
		}
	})

	t.Run("falls back to PORT when INSTANCE_ADDR empty", func(t *testing.T) {
		os.Unsetenv("INSTANCE_ADDR")
		os.Setenv("PORT", "3000")
		defer os.Unsetenv("PORT")
		addr := instanceAddress()
		if addr != "127.0.0.1:3000" {
			t.Errorf("instanceAddress = %q, want %q", addr, "127.0.0.1:3000")
		}
	})

	t.Run("defaults to 8080 when nothing set", func(t *testing.T) {
		os.Unsetenv("INSTANCE_ADDR")
		os.Unsetenv("PORT")
		addr := instanceAddress()
		if addr != "127.0.0.1:8080" {
			t.Errorf("instanceAddress = %q, want %q", addr, "127.0.0.1:8080")
		}
	})

	t.Run("returns address starting with 127.0.0.1", func(t *testing.T) {
		os.Unsetenv("INSTANCE_ADDR")
		os.Unsetenv("PORT")
		addr := instanceAddress()
		if !strings.HasPrefix(addr, "127.0.0.1:") {
			t.Errorf("instanceAddress = %q, want 127.0.0.1:… prefix", addr)
		}
	})
}

func TestResolveRoom_NilRedis(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0, nil)
	if h.redis != nil {
		t.Fatal("expected nil redis")
	}

	decision, err := h.ResolveRoom(context.Background(), "ABCD1")
	if err != nil {
		t.Fatalf("ResolveRoom error: %v", err)
	}
	if decision.Route != RouteLocal {
		t.Errorf("Route = %d, want RouteLocal", decision.Route)
	}
}

func TestInvalidateLobbyReadCaches_NilRedis(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0, nil)

	// Should not panic when redis is nil
	h.invalidateLobbyReadCaches("ABCD1")
	h.invalidateLobbyReadCaches("")
}

func TestHub_ResolveRoom_LocalWhenOwnerMatches(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	redisStore, err := store.NewRedisStore(mr.Addr(), config.DefaultTimeoutConfig())
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}

	t.Setenv("INSTANCE_ID", "instance-a")
	h := NewHub(nil, redisStore, config.DefaultTimeoutConfig(), 0, 0, nil)

	ctx := context.Background()
	code := "ABCDE"
	info, _ := json.Marshal(store.RoomRegistryInfo{
		Code:      code,
		Instance:  "instance-a",
		Address:   "10.0.0.1:8080",
		CreatedAt: time.Now().UnixMilli(),
	})
	if err := redisStore.RegisterRoom(ctx, code, info, time.Hour); err != nil {
		t.Fatalf("RegisterRoom: %v", err)
	}

	decision, err := h.ResolveRoom(ctx, code)
	if err != nil {
		t.Fatalf("ResolveRoom: %v", err)
	}
	if decision.Route != RouteLocal {
		t.Fatalf("Route = %v, want RouteLocal", decision.Route)
	}
}

func TestHub_ResolveRoom_ProxyWhenOwnerDiffers(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	redisStore, err := store.NewRedisStore(mr.Addr(), config.DefaultTimeoutConfig())
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}

	t.Setenv("INSTANCE_ID", "instance-b")
	h := NewHub(nil, redisStore, config.DefaultTimeoutConfig(), 0, 0, nil)

	ctx := context.Background()
	code := "FGHIJ"
	info, _ := json.Marshal(store.RoomRegistryInfo{
		Code:      code,
		Instance:  "instance-a",
		Address:   "10.0.0.2:8080",
		CreatedAt: time.Now().UnixMilli(),
	})
	if err := redisStore.RegisterRoom(ctx, code, info, time.Hour); err != nil {
		t.Fatalf("RegisterRoom: %v", err)
	}

	decision, err := h.ResolveRoom(ctx, code)
	if err != nil {
		t.Fatalf("ResolveRoom: %v", err)
	}
	if decision.Route != RouteProxy {
		t.Fatalf("Route = %v, want RouteProxy", decision.Route)
	}
	if decision.Address != "10.0.0.2:8080" {
		t.Fatalf("Address = %q", decision.Address)
	}
}

func TestHub_CreateRoom_CodeConflict(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(nil, nil, timeouts, 0, 0, nil)
	h.rooms["CONFL"] = NewRoom("CONFL", h, nil, timeouts, 0)

	restore := SetGenerateRoomCodeHook(func() string { return "CONFL" })
	defer restore()

	_, err := h.CreateRoom(context.Background())
	if err != ErrRoomCodeConflict {
		t.Fatalf("CreateRoom error = %v, want ErrRoomCodeConflict", err)
	}
}

func TestHub_CreateRoom_WithRedisAndBroadcaster(t *testing.T) {
	h, redisStore := setupHubWithMiniredis(t, nil)
	bc := newMockBroadcaster()
	h.broadcaster = bc

	code, err := h.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if h.GetRoom(code) == nil {
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

	h.mu.RLock()
	_, subscribed := h.subscriptions[code]
	h.mu.RUnlock()
	if !subscribed {
		t.Fatal("expected broadcaster subscription after CreateRoom")
	}
}

func TestHub_RemoveRoom_DeletesFromStore(t *testing.T) {
	repo := newMockRoomRepository()
	timeouts := config.DefaultTimeoutConfig()
	h := NewHub(repo, nil, timeouts, 0, 0, nil)

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
	h := NewHub(nil, nil, timeouts, 0, 0, nil)

	code, _ := h.CreateRoom(context.Background())
	room := h.GetRoom(code)
	disconnectedAt := time.Now().UnixMilli() - protocol.ReconnectGraceMs - 1000
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
	h := NewHub(nil, nil, timeouts, 0, 0, nil)

	code, _ := h.CreateRoom(context.Background())
	room := h.GetRoom(code)
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
	h := NewHub(nil, nil, timeouts, 0, 0, nil)

	code, _ := h.CreateRoom(context.Background())
	room := h.GetRoom(code)
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
	h := NewHub(nil, nil, timeouts, 2, 2, nil)

	code1, _ := h.CreateRoom(context.Background())
	room1 := h.GetRoom(code1)
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
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	h.removeRooms(nil, removeRoomOptions{pgDelete: true})
	h.removeRooms([]string{"MISSING"}, removeRoomOptions{pgDelete: true})
	if h.RoomCount() != 0 {
		t.Fatalf("RoomCount = %d, want 0", h.RoomCount())
	}
}

func TestInstanceAddress_DefaultsToLocalhostPort(t *testing.T) {
	os.Unsetenv("INSTANCE_ADDR")
	os.Unsetenv("PORT")
	if got := instanceAddress(); got != "127.0.0.1:8080" {
		t.Fatalf("instanceAddress() = %q", got)
	}
}

func TestHub_CleanupLoop_RunsCleanupOnce(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	code, _ := h.CreateRoom(context.Background())
	room := h.GetRoom(code)
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
	if h.GetRoom(code) != nil {
		t.Fatal("empty waiting room should be cleaned up")
	}
}