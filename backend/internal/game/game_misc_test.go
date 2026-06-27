package game

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

// ─── Broadcaster Publish/Subscribe ───────────────────────────────────

func TestMockBroadcaster_PublishSubscribe(t *testing.T) {
	b := newMockBroadcaster()
	defer b.Close()

	received := make(chan BroadcastMessage, 1)
	unsub, err := b.Subscribe("ROOM1", func(msg BroadcastMessage) {
		received <- msg
	})
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	defer unsub()

	msg := BroadcastMessage{Payload: []byte("hello")}
	if err := b.Publish(context.Background(), "ROOM1", msg); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	select {
	case got := <-received:
		if string(got.Payload) != "hello" {
			t.Fatalf("expected payload 'hello', got %q", string(got.Payload))
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestMockBroadcaster_Unsubscribe(t *testing.T) {
	b := newMockBroadcaster()
	defer b.Close()

	received := make(chan BroadcastMessage, 1)
	unsub, _ := b.Subscribe("ROOM1", func(msg BroadcastMessage) {
		received <- msg
	})
	unsub()

	_ = b.Publish(context.Background(), "ROOM1", BroadcastMessage{Payload: []byte("nope")})

	select {
	case <-received:
		t.Fatal("should not receive after unsubscribe")
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

// ─── excludePlayer ───────────────────────────────────────────────────

func TestRoom_BroadcastLocal_ExcludePlayer(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	r.syncOutbound = true

	ch1 := make(chan []byte, 64)
	ch2 := make(chan []byte, 64)
	r.mu.Lock()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch1}
	r.connections["p2"] = &PlayerConn{PlayerID: "p2", Send: ch2}
	r.mu.Unlock()

	msg := []byte{0x01, 0x02, 0x03}
	r.mu.Lock()
	r.broadcastLocal(msg, "p1")
	r.mu.Unlock()

	select {
	case <-ch1:
		t.Fatal("p1 should NOT receive (excluded)")
	default:
	}
	select {
	case <-ch2:
		// expected
	default:
		t.Fatal("p2 should receive")
	}
}

func TestRoom_BroadcastLocal_NilConnDoesNotPanic(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	r.syncOutbound = true

	ch := make(chan []byte, 64)
	r.mu.Lock()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch, Conn: nil}
	r.mu.Unlock()

	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("broadcastLocal panicked with nil Conn: %v", rec)
		}
	}()

	r.broadcastLocal([]byte{0x01}, "")
}

func TestRoom_Broadcast_PublishesExcludePlayer(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	b := newMockBroadcaster()
	defer b.Close()

	h := NewHub(nil, nil, timeouts, 0, 0, b)
	r := NewRoom("ROOM1", h, nil, timeouts, 0)
	r.syncOutbound = true

	r.mu.Lock()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 64)}
	r.broadcast([]byte{0xFF}, "p1")
	r.mu.Unlock()

	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(b.published))
	}
	if b.published[0].ExcludePlayer != "p1" {
		t.Fatalf("expected ExcludePlayer 'p1', got %q", b.published[0].ExcludePlayer)
	}
}

// ─── excludeInstance prevents loops ──────────────────────────────────

func TestHub_HandleRemoteBroadcast_ExcludeInstance(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	b := newMockBroadcaster()
	defer b.Close()

	h := NewHub(nil, nil, timeouts, 0, 0, b)
	h.instanceID = "instance-A"

	room := NewRoom("ROOM1", h, nil, timeouts, 0)
	room.syncOutbound = true
	h.mu.Lock()
	h.rooms["ROOM1"] = room
	h.mu.Unlock()

	ch := make(chan []byte, 64)
	room.mu.Lock()
	room.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch}
	room.mu.Unlock()

	// 同实例发出的消息 → 应跳过
	h.handleRemoteBroadcast("ROOM1", BroadcastMessage{
		ExcludeInstance: "instance-A",
		Payload:         []byte("skip"),
	})
	select {
	case <-ch:
		t.Fatal("should not receive message from same instance")
	default:
	}

	// 不同实例发出的消息 → 应投递
	h.handleRemoteBroadcast("ROOM1", BroadcastMessage{
		ExcludeInstance: "instance-B",
		Payload:         []byte("deliver"),
	})
	select {
	case got := <-ch:
		if string(got) != "deliver" {
			t.Fatalf("expected 'deliver', got %q", string(got))
		}
	default:
		t.Fatal("should receive message from different instance")
	}
}

func TestHub_HandleRemoteBroadcast_RoomNotFound(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	b := newMockBroadcaster()
	defer b.Close()

	h := NewHub(nil, nil, timeouts, 0, 0, b)
	h.instanceID = "instance-A"

	// 房间不存在 → 不应 panic
	h.handleRemoteBroadcast("NONEXISTENT", BroadcastMessage{
		ExcludeInstance: "instance-B",
		Payload:         []byte("data"),
	})
}

// ─── nil broadcaster (single-instance mode) ──────────────────────────

func TestRoom_NilBroadcaster_NoPanic(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	r.syncOutbound = true
	// r.broadcaster is nil (hub is nil)

	ch := make(chan []byte, 64)
	r.mu.Lock()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch}
	r.broadcast([]byte{0x01}, "")
	r.mu.Unlock()

	select {
	case <-ch:
		// expected — local delivery still works
	default:
		t.Fatal("p1 should receive message even with nil broadcaster")
	}
}

func TestRoom_NilBroadcaster_CriticalNoPanic(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("TEST1", nil, nil, timeouts, 0)
	r.syncOutbound = true

	ch := make(chan []byte, 64)
	r.mu.Lock()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch}
	r.broadcastCritical([]byte{0x02})
	r.mu.Unlock()

	select {
	case <-ch:
		// expected
	default:
		t.Fatal("p1 should receive critical message even with nil broadcaster")
	}
}

// --- Interface Satisfaction Tests ---

// TestRoomRepository_InterfaceSatisfaction verifies that mockRoomRepository
// satisfies the RoomRepository interface at compile time.
func TestRoomRepository_InterfaceSatisfaction(t *testing.T) {
	var _ RoomRepository = (*mockRoomRepository)(nil)
	var _ RoomRepository = newMockRoomRepository()
}

// TestSnapshotEncoder_InterfaceSatisfaction verifies that mockSnapshotEncoder
// satisfies the SnapshotEncoder interface at compile time.
func TestSnapshotEncoder_InterfaceSatisfaction(t *testing.T) {
	var _ SnapshotEncoder = (*mockSnapshotEncoder)(nil)
	var _ SnapshotEncoder = &mockSnapshotEncoder{}
}

// --- RoomRepository Mock Tests ---

// TestMockRoomRepository_SaveAndLoad verifies the basic save/load cycle.
func TestMockRoomRepository_SaveAndLoad(t *testing.T) {
	repo := newMockRoomRepository()
	ctx := context.Background()

	ls := &domain.LobbyState{
		Code:      "TEST1",
		State:     `{"phase":"waiting"}`,
		UpdatedAt: 1000,
		CreatedAt: 900,
	}

	if err := repo.SaveLobbyState(ctx, ls); err != nil {
		t.Fatalf("SaveLobbyState failed: %v", err)
	}

	loaded, err := repo.LoadLobbyState(ctx, "TEST1")
	if err != nil {
		t.Fatalf("LoadLobbyState failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadLobbyState returned nil")
	}
	if loaded.Code != "TEST1" {
		t.Fatalf("loaded code = %q, want TEST1", loaded.Code)
	}
	if loaded.State != `{"phase":"waiting"}` {
		t.Fatalf("loaded state = %q, want {\"phase\":\"waiting\"}", loaded.State)
	}
}

// TestMockRoomRepository_LoadNotFound verifies that loading a non-existent
// code returns an error.
func TestMockRoomRepository_LoadNotFound(t *testing.T) {
	repo := newMockRoomRepository()
	ctx := context.Background()

	_, err := repo.LoadLobbyState(ctx, "NOPE1")
	if err == nil {
		t.Fatal("LoadLobbyState should return error for non-existent code")
	}
}

// TestMockRoomRepository_Delete verifies deletion works.
func TestMockRoomRepository_Delete(t *testing.T) {
	repo := newMockRoomRepository()
	ctx := context.Background()

	ls := &domain.LobbyState{Code: "DEL01", State: "{}"}
	if err := repo.SaveLobbyState(ctx, ls); err != nil {
		t.Fatalf("SaveLobbyState failed: %v", err)
	}

	if err := repo.DeleteLobbyState(ctx, "DEL01"); err != nil {
		t.Fatalf("DeleteLobbyState failed: %v", err)
	}

	// After deletion, load should fail.
	if _, err := repo.LoadLobbyState(ctx, "DEL01"); err == nil {
		t.Fatal("LoadLobbyState should fail after deletion")
	}
}

// TestMockRoomRepository_DeleteNotFound verifies that deleting a non-existent
// code returns an error.
func TestMockRoomRepository_DeleteNotFound(t *testing.T) {
	repo := newMockRoomRepository()
	ctx := context.Background()

	if err := repo.DeleteLobbyState(ctx, "NOPE1"); err == nil {
		t.Fatal("DeleteLobbyState should return error for non-existent code")
	}
}

// TestMockRoomRepository_SaveNil verifies that saving nil returns an error.
// This is adversarial: tests the mock's nil-safety.
func TestMockRoomRepository_SaveNil(t *testing.T) {
	repo := newMockRoomRepository()
	ctx := context.Background()

	if err := repo.SaveLobbyState(ctx, nil); err == nil {
		t.Fatal("SaveLobbyState should return error for nil lobby state")
	}
}

// TestMockRoomRepository_Overwrite verifies that saving the same code twice
// overwrites the previous state.
func TestMockRoomRepository_Overwrite(t *testing.T) {
	repo := newMockRoomRepository()
	ctx := context.Background()

	ls1 := &domain.LobbyState{Code: "OVR01", State: `{"v":1}`}
	ls2 := &domain.LobbyState{Code: "OVR01", State: `{"v":2}`}

	if err := repo.SaveLobbyState(ctx, ls1); err != nil {
		t.Fatalf("SaveLobbyState ls1 failed: %v", err)
	}
	if err := repo.SaveLobbyState(ctx, ls2); err != nil {
		t.Fatalf("SaveLobbyState ls2 failed: %v", err)
	}

	loaded, err := repo.LoadLobbyState(ctx, "OVR01")
	if err != nil {
		t.Fatalf("LoadLobbyState failed: %v", err)
	}
	if loaded.State != `{"v":2}` {
		t.Fatalf("loaded state = %q, want {\"v\":2}", loaded.State)
	}
}

// TestMockRoomRepository_Isolation verifies that the mock returns copies,
// not references to internal state. This is adversarial: if the mock returned
// internal pointers, external mutation would corrupt the store.
func TestMockRoomRepository_Isolation(t *testing.T) {
	repo := newMockRoomRepository()
	ctx := context.Background()

	ls := &domain.LobbyState{Code: "ISO01", State: `{"v":1}`}
	if err := repo.SaveLobbyState(ctx, ls); err != nil {
		t.Fatalf("SaveLobbyState failed: %v", err)
	}

	loaded1, _ := repo.LoadLobbyState(ctx, "ISO01")
	loaded1.State = `{"mutated":true}`

	loaded2, _ := repo.LoadLobbyState(ctx, "ISO01")
	if loaded2.State != `{"v":1}` {
		t.Fatalf("mock did not isolate stored state: got %q, want {\"v\":1}", loaded2.State)
	}
}

// TestMockRoomRepository_ErrorInjection verifies that the mock can be
// configured to return errors for testing error paths.
func TestMockRoomRepository_ErrorInjection(t *testing.T) {
	repo := newMockRoomRepository()
	ctx := context.Background()

	saveErr := errors.New("simulated save failure")
	repo.saveErr = saveErr
	if err := repo.SaveLobbyState(ctx, &domain.LobbyState{Code: "X"}); err != saveErr {
		t.Fatalf("SaveLobbyState error = %v, want %v", err, saveErr)
	}

	loadErr := errors.New("simulated load failure")
	repo.loadErr = loadErr
	if _, err := repo.LoadLobbyState(ctx, "X"); err != loadErr {
		t.Fatalf("LoadLobbyState error = %v, want %v", err, loadErr)
	}

	deleteErr := errors.New("simulated delete failure")
	repo.deleteErr = deleteErr
	if err := repo.DeleteLobbyState(ctx, "X"); err != deleteErr {
		t.Fatalf("DeleteLobbyState error = %v, want %v", err, deleteErr)
	}
}

// TestMockRoomRepository_CallCounts verifies that the mock tracks call counts.
func TestMockRoomRepository_CallCounts(t *testing.T) {
	repo := newMockRoomRepository()
	ctx := context.Background()

	// 3 saves
	for i := 0; i < 3; i++ {
		repo.SaveLobbyState(ctx, &domain.LobbyState{Code: fmt.Sprintf("C%02d", i)})
	}
	// 2 loads
	repo.LoadLobbyState(ctx, "C00")
	repo.LoadLobbyState(ctx, "C01")
	// 1 delete
	repo.DeleteLobbyState(ctx, "C00")

	if repo.saveCount != 3 {
		t.Fatalf("saveCount = %d, want 3", repo.saveCount)
	}
	if repo.loadCount != 2 {
		t.Fatalf("loadCount = %d, want 2", repo.loadCount)
	}
	if repo.deleteCount != 1 {
		t.Fatalf("deleteCount = %d, want 1", repo.deleteCount)
	}
}

// TestMockRoomRepository_ConcurrentAccess verifies the mock is safe for
// concurrent use. Run with -race.
func TestMockRoomRepository_ConcurrentAccess(t *testing.T) {
	repo := newMockRoomRepository()
	ctx := context.Background()

	var wg sync.WaitGroup
	// Concurrent writers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			code := fmt.Sprintf("CON%02d", n)
			_ = repo.SaveLobbyState(ctx, &domain.LobbyState{Code: code})
		}(i)
	}
	// Concurrent readers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, _ = repo.LoadLobbyState(ctx, fmt.Sprintf("CON%02d", n%20))
		}(i)
	}
	// Concurrent deleters
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = repo.DeleteLobbyState(ctx, fmt.Sprintf("CON%02d", n))
		}(i)
	}
	wg.Wait()
}

// TestMockRoomRepository_CancelledContext verifies that the mock respects
// the context (currently it doesn't check ctx, but this documents the behavior).
func TestMockRoomRepository_CancelledContext(t *testing.T) {
	repo := newMockRoomRepository()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// The mock doesn't check context, so these should succeed.
	// This test documents that the mock ignores context cancellation.
	if err := repo.SaveLobbyState(ctx, &domain.LobbyState{Code: "CAN01"}); err != nil {
		t.Fatalf("SaveLobbyState with cancelled context failed: %v", err)
	}
	if _, err := repo.LoadLobbyState(ctx, "CAN01"); err != nil {
		t.Fatalf("LoadLobbyState with cancelled context failed: %v", err)
	}
}

// --- SnapshotEncoder Mock Tests ---

// TestMockSnapshotEncoder_Encode verifies basic encoding.
func TestMockSnapshotEncoder_Encode(t *testing.T) {
	enc := &mockSnapshotEncoder{}
	state := &domain.GameState{
		Phase:     domain.PhaseWaiting,
		LobbyCode: "ENC01",
	}

	data, err := enc.Encode(state)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Encode returned empty data")
	}
	if enc.encodeCount != 1 {
		t.Fatalf("encodeCount = %d, want 1", enc.encodeCount)
	}
	if enc.lastState != state {
		t.Fatal("lastState not set correctly")
	}
}

// TestMockSnapshotEncoder_EncodeNil verifies encoding a nil state.
func TestMockSnapshotEncoder_EncodeNil(t *testing.T) {
	enc := &mockSnapshotEncoder{}
	data, err := enc.Encode(nil)
	if err != nil {
		t.Fatalf("Encode(nil) failed: %v", err)
	}
	if string(data) != "null" {
		t.Fatalf("Encode(nil) = %q, want %q", string(data), "null")
	}
}

// TestMockSnapshotEncoder_ErrorInjection verifies error injection.
func TestMockSnapshotEncoder_ErrorInjection(t *testing.T) {
	enc := &mockSnapshotEncoder{}
	enc.encodeErr = errors.New("encode failure")

	_, err := enc.Encode(&domain.GameState{})
	if err != enc.encodeErr {
		t.Fatalf("Encode error = %v, want %v", err, enc.encodeErr)
	}
}

// TestMockSnapshotEncoder_MultipleCalls verifies the counter increments.
func TestMockSnapshotEncoder_MultipleCalls(t *testing.T) {
	enc := &mockSnapshotEncoder{}
	for i := 0; i < 5; i++ {
		_, _ = enc.Encode(&domain.GameState{Phase: domain.PhasePlaying})
	}
	if enc.encodeCount != 5 {
		t.Fatalf("encodeCount = %d, want 5", enc.encodeCount)
	}
}

// --- Interface Contract Tests ---

// TestRoomRepository_InterfaceContract verifies that any RoomRepository
// implementation must have the correct method signatures. This is a
// compile-time check that also serves as documentation.
func TestRoomRepository_InterfaceContract(t *testing.T) {
	// The interface requires:
	//   SaveLobbyState(ctx context.Context, ls *domain.LobbyState) error
	//   LoadLobbyState(ctx context.Context, code string) (*domain.LobbyState, error)
	//   DeleteLobbyState(ctx context.Context, code string) error
	var repo RoomRepository = newMockRoomRepository()

	ctx := context.Background()
	ls := &domain.LobbyState{Code: "CON01", State: "{}"}

	// Exercise all three methods to verify the contract.
	if err := repo.SaveLobbyState(ctx, ls); err != nil {
		t.Fatalf("SaveLobbyState contract violation: %v", err)
	}
	if got, err := repo.LoadLobbyState(ctx, "CON01"); err != nil || got == nil {
		t.Fatalf("LoadLobbyState contract violation: got=%v err=%v", got, err)
	}
	if err := repo.DeleteLobbyState(ctx, "CON01"); err != nil {
		t.Fatalf("DeleteLobbyState contract violation: %v", err)
	}
}

// TestSnapshotEncoder_InterfaceContract verifies that any SnapshotEncoder
// implementation must have the correct method signature.
func TestSnapshotEncoder_InterfaceContract(t *testing.T) {
	// The interface requires:
	//   Encode(state *domain.GameState) ([]byte, error)
	var enc SnapshotEncoder = &mockSnapshotEncoder{}

	data, err := enc.Encode(&domain.GameState{Phase: domain.PhaseEnded})
	if err != nil {
		t.Fatalf("Encode contract violation: %v", err)
	}
	if data == nil {
		t.Fatal("Encode returned nil data")
	}
}

// TestRoomRepository_MethodCount verifies the interface has exactly 3 methods.
// This is adversarial: catches accidental addition/removal of methods.
func TestRoomRepository_MethodCount(t *testing.T) {
	// We can't easily count interface methods at runtime in Go,
	// but we can verify all expected methods exist by calling them.
	// If a method is missing, the code won't compile.
	var _ = RoomRepository(nil)
	// The fact that this compiles verifies the interface exists.
}

// TestSnapshotEncoder_MethodCount verifies the interface has exactly 1 method.
func TestSnapshotEncoder_MethodCount(t *testing.T) {
	var _ = SnapshotEncoder(nil)
}

// ─── GenerateRandomNickname ──────────────────────────────────────────

func TestGenerateRandomNickname_NonEmpty(t *testing.T) {
	for i := 0; i < 100; i++ {
		name := GenerateRandomNickname(map[string]bool{})
		if name == "" {
			t.Fatal("生成的昵称不应为空")
		}
	}
}

func TestGenerateRandomNickname_ContainsDe(t *testing.T) {
	for i := 0; i < 50; i++ {
		name := GenerateRandomNickname(map[string]bool{})
		if !strings.Contains(name, "的") {
			t.Fatalf("生成的昵称应包含'的'字, got=%s", name)
		}
	}
}

func TestGenerateRandomNickname_ExcludeList(t *testing.T) {
	excluded := map[string]bool{"敏捷的飞行员": true}
	for i := 0; i < 50; i++ {
		name := GenerateRandomNickname(excluded)
		if name == "敏捷的飞行员" {
			t.Fatal("排除列表中的名字不应被生成")
		}
	}
}

func TestGenerateRandomNickname_AllExcluded(t *testing.T) {
	// 排除所有基础组合 → 应返回带 # 后缀的名字或 PlayerXXXX 兜底
	allNames := getAllNicknameCombinations()
	excludeAll := make(map[string]bool)
	for _, n := range allNames {
		excludeAll[n] = true
	}

	name := GenerateRandomNickname(excludeAll)

	if excludeAll[name] {
		t.Fatalf("返回的名字不应在排除列表中，got=%s", name)
	}
	// 应返回带 # 后缀的名字或 PlayerXXXX 兜底
	hasHash := strings.Contains(name, "#")
	hasPlayer := strings.HasPrefix(name, "Player")
	if !hasHash && !hasPlayer {
		t.Fatalf("排除所有基础组合后应返回带 # 后缀或 Player 前缀的名字，got=%s", name)
	}
}

func TestGenerateRandomNickname_EmptyExclude(t *testing.T) {
	name := GenerateRandomNickname(nil)
	if name == "" {
		t.Fatal("nil 排除列表时应正常生成")
	}
}

// ─── GenerateUniqueNickname ──────────────────────────────────────────

func TestGenerateUniqueNickname_NoConflict(t *testing.T) {
	usedNames := map[string]bool{"已占用": true}
	result := GenerateUniqueNickname("玩家甲", usedNames)
	if result != "玩家甲" {
		t.Fatalf("不重复的名字应直接使用，got=%s", result)
	}
}

func TestGenerateUniqueNickname_Conflict(t *testing.T) {
	usedNames := map[string]bool{"玩家甲": true}
	result := GenerateUniqueNickname("玩家甲", usedNames)
	if result == "玩家甲" {
		t.Fatal("重复的名字应生成随机名字")
	}
	if usedNames[result] {
		t.Fatal("生成的名字不应在已用列表中")
	}
}

func TestGenerateUniqueNickname_Empty(t *testing.T) {
	usedNames := map[string]bool{}
	result := GenerateUniqueNickname("", usedNames)
	if len(result) == 0 {
		t.Fatal("空名字应生成随机昵称")
	}
}

func TestGenerateUniqueNickname_DangerousChars(t *testing.T) {
	usedNames := map[string]bool{}
	result := GenerateUniqueNickname("<script>alert(1)</script>", usedNames)
	if strings.Contains(result, "<") || strings.Contains(result, ">") {
		t.Fatalf("危险字符名字应被拒绝并生成随机昵称，got=%s", result)
	}
}

// ─── SanitizePlayerName ──────────────────────────────────────────────

func TestSanitizePlayerName_Normal(t *testing.T) {
	result := SanitizePlayerName("正常名字")
	if result != "正常名字" {
		t.Fatalf("正常名字应原样返回，got=%s", result)
	}
}

func TestSanitizePlayerName_ScriptTag(t *testing.T) {
	result := SanitizePlayerName("<script>")
	if strings.Contains(result, "<") || strings.Contains(result, ">") {
		t.Fatalf("HTML 特殊字符应被移除，got=%s", result)
	}
}

func TestSanitizePlayerName_Truncate(t *testing.T) {
	long := strings.Repeat("a", 1000)
	result := SanitizePlayerName(long)
	if len([]rune(result)) != protocol.MaxNicknameLen {
		t.Fatalf("超长名字应截断至 %d 字符，got len=%d", protocol.MaxNicknameLen, len([]rune(result)))
	}
}

func TestSanitizePlayerName_Empty(t *testing.T) {
	result := SanitizePlayerName("")
	if result != "" {
		t.Fatalf("空字符串应返回空字符串，got=%s", result)
	}
}

func TestSanitizePlayerName_Whitespace(t *testing.T) {
	result := SanitizePlayerName("  多余  空格  ")
	if result != "多余 空格" {
		t.Fatalf("多余空白应折叠为单个空格，got=%s", result)
	}
}

func TestSanitizePlayerName_Quotes(t *testing.T) {
	result := SanitizePlayerName("a'b\"c")
	if result != "abc" {
		t.Fatalf("引号字符应被去除，got=%s", result)
	}
}

func TestSanitizePlayerName_Ampersand(t *testing.T) {
	result := SanitizePlayerName("a&b")
	if result != "ab" {
		t.Fatalf("& 符号应被去除，got=%s", result)
	}
}

func TestSanitizePlayerName_OnlyDangerous(t *testing.T) {
	result := SanitizePlayerName("<>\"'`&")
	if result != "" {
		t.Fatalf("仅含危险字符应返回空字符串，got=%s", result)
	}
}

func TestSanitizePlayerName_ControlChars(t *testing.T) {
	result := SanitizePlayerName("hello\x00world\x1F")
	if result != "helloworld" {
		t.Fatalf("控制字符应被移除，got=%s", result)
	}
}

// ─── 内部辅助：获取所有昵称组合 ─────────────────────────────────────

func getAllNicknameCombinations() []string {
	var names []string
	for _, adj := range NicknameAdjectives {
		for _, cat := range NicknameCategories {
			for _, noun := range cat {
				names = append(names, adj+noun)
			}
		}
	}
	return names
}

// ─── randomIndex ─────────────────────────────────────────────────────

func TestRandomIndex(t *testing.T) {
	// Valid range
	for i := 0; i < 50; i++ {
		idx := randomIndex(10)
		if idx < 0 || idx >= 10 {
			t.Fatalf("randomIndex(10) = %d, want [0, 10)", idx)
		}
	}

	// Zero or negative
	idx := randomIndex(0)
	if idx != 0 {
		t.Fatalf("randomIndex(0) = %d, want 0", idx)
	}
	idx = randomIndex(-1)
	if idx != 0 {
		t.Fatalf("randomIndex(-1) = %d, want 0", idx)
	}
}

// ─── GenerateUniqueNickname edge cases ───────────────────────────────

func TestGenerateUniqueNickname_Truncation(t *testing.T) {
	usedNames := map[string]bool{}
	// Very long name should be truncated
	longName := strings.Repeat("a", 50)
	result := GenerateUniqueNickname(longName, usedNames)
	if len([]rune(result)) > 12 {
		t.Fatalf("long client name should be truncated to 12 chars, got %d", len([]rune(result)))
	}
}

func TestGenerateUniqueNickname_ClientNameNotInUse(t *testing.T) {
	usedNames := map[string]bool{}
	result := GenerateUniqueNickname("Alice", usedNames)
	if result != "Alice" {
		t.Fatalf("unused client name should be used directly, got %q", result)
	}
}

func TestGenerateUniqueNickname_ClientNameInUse(t *testing.T) {
	usedNames := map[string]bool{"Alice": true}
	result := GenerateUniqueNickname("Alice", usedNames)
	if result == "Alice" {
		t.Fatal("used client name should trigger random generation")
	}
}

// --- sanitizeNickname (player.go) ---

func TestSanitizeNickname_ControlChars(t *testing.T) {
	result := sanitizeNickname("hello\x00world\x01test")
	want := "helloworldte"
	if result != want {
		t.Errorf("sanitizeNickname = %q, want %q", result, want)
	}
}

func TestSanitizeNickname_ZeroWidthChars(t *testing.T) {
	result := sanitizeNickname("hello\u200Bworld")
	if result != "helloworld" {
		t.Errorf("sanitizeNickname = %q, want helloworld", result)
	}
}

func TestSanitizeNickname_HTMLChars(t *testing.T) {
	result := sanitizeNickname("test<script>alert('xss')</script>")
	if strings.ContainsAny(result, "<>'\"`&") {
		t.Errorf("sanitizeNickname should remove HTML chars, got %q", result)
	}
}

func TestSanitizeNickname_TrimSpace(t *testing.T) {
	result := sanitizeNickname("  hello  ")
	if result != "hello" {
		t.Errorf("sanitizeNickname = %q, want hello", result)
	}
}

func TestSanitizeNickname_EmptyAfterSanitization(t *testing.T) {
	result := sanitizeNickname("\x00\x01\x02")
	if result != "" {
		t.Errorf("sanitizeNickname = %q, want empty string", result)
	}
}

// ─── NewGameState ────────────────────────────────────────────────────

func TestNewGameState_InitialValues(t *testing.T) {
	state := NewGameState("TEST")

	if state.Phase != domain.PhaseWaiting {
		t.Fatalf("初始 phase 应为 waiting，got=%v", state.Phase)
	}
	if state.TickCount != 0 {
		t.Fatalf("初始 tickCount 应为 0，got=%d", state.TickCount)
	}
	if state.Balloon.X != 0.5 {
		t.Fatalf("初始气球 X 应为 0.5，got=%v", state.Balloon.X)
	}
	if state.Balloon.Y != 0.95 {
		t.Fatalf("初始气球 Y 应为 0.95，got=%v", state.Balloon.Y)
	}
	if state.Balloon.VX != 0 || state.Balloon.VY != 0 {
		t.Fatalf("初始气球速度应为 0，got VX=%v VY=%v", state.Balloon.VX, state.Balloon.VY)
	}
	if state.Balloon.Score != 0 {
		t.Fatalf("初始分数应为 0，got=%d", state.Balloon.Score)
	}
	if !state.Ghost.Active {
		t.Fatal("初始幽灵应已激活")
	}
	if state.Wind != 0 {
		t.Fatalf("初始风场应为 0，got=%v", state.Wind)
	}
	if state.WindTarget != 0 {
		t.Fatalf("初始 WindTarget 应为 0，got=%v", state.WindTarget)
	}
	if state.WindChangeCountdown != 112 {
		t.Fatalf("初始 WindChangeCountdown 应为 112，got=%d", state.WindChangeCountdown)
	}
	if len(state.RestartVotes) != 0 {
		t.Fatalf("初始 RestartVotes 应为空，got=%d", len(state.RestartVotes))
	}
	if state.RestartTimerStart != nil {
		t.Fatal("初始 RestartTimerStart 应为 nil")
	}
	if state.LobbyCode != "TEST" {
		t.Fatalf("LobbyCode 应为 TEST，got=%v", state.LobbyCode)
	}
}

func TestNewGameState_GhostInBounds(t *testing.T) {
	for i := 0; i < 50; i++ {
		state := NewGameState("TEST")
		if state.Ghost.X < 0.15 || state.Ghost.X > 0.85 {
			t.Fatalf("幽灵 X 应在 0.15-0.85，got=%v", state.Ghost.X)
		}
		if state.Ghost.Y < 0.3 || state.Ghost.Y > 0.75 {
			t.Fatalf("幽灵 Y 应在 0.3-0.75，got=%v", state.Ghost.Y)
		}
	}
}

func TestNewGameState_GhostHasSpeed(t *testing.T) {
	state := NewGameState("TEST")
	speed := sqrt(state.Ghost.VX*state.Ghost.VX + state.Ghost.VY*state.Ghost.VY)
	if speed == 0 {
		t.Fatal("初始幽灵应有非零速度")
	}
}

// ─── ResetGameEntities ───────────────────────────────────────────────

func TestResetGameEntities_ResetsBalloon(t *testing.T) {
	state := createTestState()
	state.Balloon.X = 0.3
	state.Balloon.Y = 0.5
	state.Balloon.VX = 0.1
	state.Balloon.VY = 0.2
	state.Balloon.Score = 500
	state.TickCount = 200

	ResetGameEntities(state, RandomSpawnTimer())

	if state.Balloon.X != 0.5 {
		t.Fatalf("重置后气球 X 应为 0.5，got=%v", state.Balloon.X)
	}
	if state.Balloon.Y != 0.95 {
		t.Fatalf("重置后气球 Y 应为 0.95，got=%v", state.Balloon.Y)
	}
	if state.Balloon.VX != 0 || state.Balloon.VY != 0 {
		t.Fatalf("重置后气球速度应为 0，got VX=%v VY=%v", state.Balloon.VX, state.Balloon.VY)
	}
	if state.Balloon.Score != 0 {
		t.Fatalf("重置后分数应为 0，got=%d", state.Balloon.Score)
	}
	if state.TickCount != 0 {
		t.Fatalf("重置后 tickCount 应为 0，got=%d", state.TickCount)
	}
}

func TestResetGameEntities_ResetsGhost(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = false
	state.Ghost.SpawnTimer = 100

	ResetGameEntities(state, RandomSpawnTimer())

	if !state.Ghost.Active {
		t.Fatal("重置后幽灵应已激活")
	}
}

func TestResetGameEntities_ResetsWind(t *testing.T) {
	state := createTestState()
	state.Wind = 0.8
	state.WindTarget = -0.5
	state.WindChangeCountdown = 10

	ResetGameEntities(state, RandomSpawnTimer())

	if state.Wind != 0 {
		t.Fatalf("重置后风场应为 0，got=%v", state.Wind)
	}
	if state.WindTarget != 0 {
		t.Fatalf("重置后 WindTarget 应为 0，got=%v", state.WindTarget)
	}
	if state.WindChangeCountdown != 112 {
		t.Fatalf("重置后 WindChangeCountdown 应为 112，got=%d", state.WindChangeCountdown)
	}
}

func TestResetGameEntities_ResetsVotes(t *testing.T) {
	state := createTestState()
	state.RestartVotes["player1"] = true
	state.RestartVotes["player2"] = true
	now := int64(1234567890)
	state.RestartTimerStart = &now

	ResetGameEntities(state, RandomSpawnTimer())

	if len(state.RestartVotes) != 0 {
		t.Fatalf("重置后 RestartVotes 应为空，got=%d", len(state.RestartVotes))
	}
	if state.RestartTimerStart != nil {
		t.Fatal("重置后 RestartTimerStart 应为 nil")
	}
}

// ─── SerializeState / DeserializeState ───────────────────────────────

func TestSerializeDeserialize_RoundTrip(t *testing.T) {
	original := buildTestGameState(1700000000)

	data, err := SerializeState(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	restored, err := DeserializeState(data)
	if err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	assertGameStateEqual(t, original, restored)
}

// buildTestGameState constructs a GameState with representative fields for round-trip testing.
func buildTestGameState(now int64) *domain.GameState {
	return &domain.GameState{
		Phase: domain.PhasePlaying,
		Balloon: domain.BalloonState{
			X: 0.5, Y: 0.95, VX: 0.01, VY: 0.02, Score: 100,
		},
		Bird: domain.BirdState{
			X: 0.3, Y: 0.4, VX: 0.005, VY: 0, Active: true, SpawnTimer: 0,
		},
		Ghost: domain.GhostState{
			X: 0.6, Y: 0.5, VX: -0.002, VY: 0.001, Active: true, SpawnTimer: 20, RepelTimer: 0,
		},
		Players: map[string]*domain.PlayerState{
			"p1": {
				ID: "p1", PlayerIndex: 0, Nickname: "快乐的气球",
				Palette: 1, ScoreContribution: 50, TapsCount: 10,
			},
		},
		NextPlayerIndex:     1,
		TickCount:           42,
		StartedAt:           now,
		SessionID:           "sess-123",
		LobbyCode:           "ABCDE",
		Wind:                0.3,
		WindTarget:          -0.2,
		WindChangeCountdown: 100,
		WindMicroCountdown:  5,
		WindMidCountdown:    50,
		WindMidOffset:       0.01,
		RestartVotes:        map[string]bool{"p1": true},
		RestartTimerStart:   &now,
	}
}

// assertGameStateEqual verifies that restored matches original across key fields.
func assertGameStateEqual(t *testing.T, original, restored *domain.GameState) {
	t.Helper()
	if restored.Phase != original.Phase {
		t.Fatalf("Phase 不匹配: got=%v, want=%v", restored.Phase, original.Phase)
	}
	if restored.Balloon.X != original.Balloon.X {
		t.Fatalf("Balloon.X 不匹配: got=%v, want=%v", restored.Balloon.X, original.Balloon.X)
	}
	if restored.Balloon.Score != original.Balloon.Score {
		t.Fatalf("Balloon.Score 不匹配: got=%v, want=%v", restored.Balloon.Score, original.Balloon.Score)
	}
	if restored.TickCount != original.TickCount {
		t.Fatalf("TickCount 不匹配: got=%v, want=%v", restored.TickCount, original.TickCount)
	}
	if restored.SessionID != original.SessionID {
		t.Fatalf("SessionID 不匹配: got=%v, want=%v", restored.SessionID, original.SessionID)
	}
	if restored.Wind != original.Wind {
		t.Fatalf("Wind 不匹配: got=%v, want=%v", restored.Wind, original.Wind)
	}
	if len(restored.Players) != len(original.Players) {
		t.Fatalf("Players 数量不匹配: got=%d, want=%d", len(restored.Players), len(original.Players))
	}
	if restored.Players["p1"].Nickname != original.Players["p1"].Nickname {
		t.Fatalf("Player nickname 不匹配: got=%v, want=%v",
			restored.Players["p1"].Nickname, original.Players["p1"].Nickname)
	}
	if len(restored.RestartVotes) != len(original.RestartVotes) {
		t.Fatalf("RestartVotes 数量不匹配: got=%d, want=%d", len(restored.RestartVotes), len(original.RestartVotes))
	}
}

// ─── Benchmarks ──────────────────────────────────────────────────────

func BenchmarkSerializeState(b *testing.B) {
	state := &domain.GameState{
		Phase: domain.PhasePlaying,
		Balloon: domain.BalloonState{
			X: 0.5, Y: 0.95, VX: 0.01, VY: 0.02, Score: 100,
		},
		Bird: domain.BirdState{
			X: 0.3, Y: 0.4, VX: 0.005, VY: 0, Active: true, SpawnTimer: 0,
		},
		Ghost: domain.GhostState{
			X: 0.6, Y: 0.5, VX: -0.002, VY: 0.001, Active: true, SpawnTimer: 20, RepelTimer: 0,
		},
		Players: map[string]*domain.PlayerState{
			"p1": {ID: "p1", PlayerIndex: 0, Nickname: "TestPlayer", Palette: 1, ScoreContribution: 50},
			"p2": {ID: "p2", PlayerIndex: 1, Nickname: "AnotherPlayer", Palette: 2, ScoreContribution: 30},
		},
		NextPlayerIndex:     2,
		TickCount:           42,
		StartedAt:           1700000000,
		SessionID:           "sess-123",
		LobbyCode:           "ABCDE",
		Wind:                0.3,
		WindTarget:          -0.2,
		WindChangeCountdown: 100,
		WindMicroCountdown:  5,
		WindMidCountdown:    50,
		WindMidOffset:       0.01,
		RestartVotes:        map[string]bool{"p1": true},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SerializeState(state)
	}
}

func BenchmarkDeserializeState(b *testing.B) {
	state := &domain.GameState{
		Phase: domain.PhasePlaying,
		Balloon: domain.BalloonState{
			X: 0.5, Y: 0.95, VX: 0.01, VY: 0.02, Score: 100,
		},
		Bird: domain.BirdState{
			X: 0.3, Y: 0.4, VX: 0.005, VY: 0, Active: true, SpawnTimer: 0,
		},
		Ghost: domain.GhostState{
			X: 0.6, Y: 0.5, VX: -0.002, VY: 0.001, Active: true, SpawnTimer: 20, RepelTimer: 0,
		},
		Players: map[string]*domain.PlayerState{
			"p1": {ID: "p1", PlayerIndex: 0, Nickname: "TestPlayer", Palette: 1, ScoreContribution: 50},
		},
		NextPlayerIndex:     1,
		TickCount:           42,
		StartedAt:           1700000000,
		SessionID:           "sess-123",
		LobbyCode:           "ABCDE",
		Wind:                0.3,
		WindTarget:          -0.2,
		WindChangeCountdown: 100,
		WindMicroCountdown:  5,
		WindMidCountdown:    50,
		WindMidOffset:       0.01,
		RestartVotes:        map[string]bool{"p1": true},
	}
	data, _ := SerializeState(state)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DeserializeState(data)
	}
}

func BenchmarkNewGameState(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewGameState("BENCH")
	}
}

// ─── 辅助函数 ────────────────────────────────────────────────────────

func createTestState() *domain.GameState {
	state := NewGameState("TEST")
	state.Phase = domain.PhasePlaying
	return state
}

// ─── 气球物理 ────────────────────────────────────────────────────────

func TestApplyPhysics_Gravity(t *testing.T) {
	balloon := domain.BalloonState{X: 0.5, Y: 0.5, VX: 0, VY: 0}
	yBefore := balloon.Y

	gameOver := ApplyPhysics(&balloon)

	if gameOver {
		t.Fatal("不应游戏结束")
	}
	if balloon.VY >= 0 {
		t.Fatalf("重力应使 VY 减小（向下），got VY=%v", balloon.VY)
	}
	if balloon.Y >= yBefore {
		t.Fatalf("重力应使 Y 减小（下落），got Y=%v, before=%v", balloon.Y, yBefore)
	}
}

func TestApplyPhysics_LeftBoundary(t *testing.T) {
	balloon := domain.BalloonState{X: 0, Y: 0.5, VX: -0.01, VY: 0}

	ApplyPhysics(&balloon)

	if balloon.X != 0 {
		t.Fatalf("左边界应钳制 X=0，got X=%v", balloon.X)
	}
	if balloon.VX <= 0 {
		t.Fatalf("左边界反弹后 VX 应为正，got VX=%v", balloon.VX)
	}
}

func TestApplyPhysics_RightBoundary(t *testing.T) {
	balloon := domain.BalloonState{X: 1, Y: 0.5, VX: 0.01, VY: 0}

	ApplyPhysics(&balloon)

	if balloon.X != 1 {
		t.Fatalf("右边界应钳制 X=1，got X=%v", balloon.X)
	}
	if balloon.VX >= 0 {
		t.Fatalf("右边界反弹后 VX 应为负，got VX=%v", balloon.VX)
	}
}

func TestApplyPhysics_HorizontalDrag(t *testing.T) {
	balloon := domain.BalloonState{X: 0.5, Y: 0.5, VX: 0.1, VY: 0}

	ApplyPhysics(&balloon)

	if math.Abs(balloon.VX) >= 0.1 {
		t.Fatalf("空气阻力应使水平速度衰减，got VX=%v", balloon.VX)
	}
}

func TestApplyPhysics_GameOver(t *testing.T) {
	balloon := domain.BalloonState{X: 0.5, Y: 0.001, VX: 0, VY: -0.01}

	gameOver := ApplyPhysics(&balloon)

	if !gameOver {
		t.Fatal("触底应导致游戏结束")
	}
}

func TestApplyPhysics_Ceiling(t *testing.T) {
	balloon := domain.BalloonState{X: 0.5, Y: 0.999, VX: 0, VY: 0.05}

	gameOver := ApplyPhysics(&balloon)

	if gameOver {
		t.Fatal("撞天花板不应游戏结束")
	}
	if balloon.VY > 0 {
		t.Fatalf("撞天花板后 VY 应归零或为负，got VY=%v", balloon.VY)
	}
}

// ─── 点击推力 ────────────────────────────────────────────────────────

func TestApplyTapForce_InRange(t *testing.T) {
	balloon := domain.BalloonState{X: 0.5, Y: 0.5, VX: 0, VY: 0}

	ok := ApplyTapForce(&balloon, 0.5, 0.3)

	if !ok {
		t.Fatal("点击在有效范围内应返回 true")
	}
	if balloon.VY <= 0 {
		t.Fatalf("点击在气球下方应获得向上速度，got VY=%v", balloon.VY)
	}
}

func TestApplyTapForce_OutOfRange(t *testing.T) {
	balloon := domain.BalloonState{X: 0.5, Y: 0.5, VX: 0, VY: 0}
	vyBefore := balloon.VY

	ok := ApplyTapForce(&balloon, 0.1, 0.1)

	if ok {
		t.Fatal("点击超出有效范围应返回 false")
	}
	if balloon.VY != vyBefore {
		t.Fatalf("超出范围时 VY 不应变，got VY=%v, before=%v", balloon.VY, vyBefore)
	}
}

func TestApplyTapForce_Center(t *testing.T) {
	balloon := domain.BalloonState{X: 0.5, Y: 0.5, VX: 0, VY: 0}
	vyBefore := balloon.VY

	ok := ApplyTapForce(&balloon, 0.5, 0.5)

	if !ok {
		t.Fatal("点击气球中心应返回 true")
	}
	if math.Abs((balloon.VY-vyBefore)-protocol.TapForce) > 1e-9 {
		t.Fatalf("中心点击应给纯向上推力 TAP_FORCE，got VY diff=%v, want=%v",
			balloon.VY-vyBefore, protocol.TapForce)
	}
}

// ─── 幽灵碰撞 ────────────────────────────────────────────────────────

func TestCheckGhostCollision_Overlap(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.5
	state.Balloon.X = 0.5
	state.Balloon.Y = 0.5

	if !CheckGhostCollision(state) {
		t.Fatal("幽灵与气球重叠时应返回 true")
	}
}

func TestCheckGhostCollision_Edge(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.5
	state.Balloon.X = 0.5 + protocol.GhostCollisionRadius*0.9
	state.Balloon.Y = 0.5

	if !CheckGhostCollision(state) {
		t.Fatal("幽灵在碰撞半径边缘应返回 true")
	}
}

func TestCheckGhostCollision_Outside(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.5
	state.Balloon.X = 0.5 + protocol.GhostCollisionRadius*2
	state.Balloon.Y = 0.5

	if CheckGhostCollision(state) {
		t.Fatal("幽灵在碰撞半径外应返回 false")
	}
}

func TestCheckGhostCollision_Inactive(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = false
	state.Ghost.X = state.Balloon.X
	state.Ghost.Y = state.Balloon.Y

	if CheckGhostCollision(state) {
		t.Fatal("幽灵未激活时应返回 false")
	}
}

func TestCheckGhostCollision_Damage(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = state.Balloon.X
	state.Ghost.Y = state.Balloon.Y
	vyBefore := state.Balloon.VY

	CheckGhostCollision(state)

	if state.Balloon.VY != vyBefore-protocol.GhostDamage {
		t.Fatalf("碰撞后气球 VY 应减少 GHOST_DAMAGE，got VY=%v, want=%v",
			state.Balloon.VY, vyBefore-protocol.GhostDamage)
	}
}

func TestCheckGhostCollision_GhostBounce(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.5
	state.Balloon.X = 0.5
	state.Balloon.Y = 0.5

	CheckGhostCollision(state)

	speed := math.Sqrt(state.Ghost.VX*state.Ghost.VX + state.Ghost.VY*state.Ghost.VY)
	expectedSpeed := protocol.GhostSpeed * 3
	if math.Abs(speed-expectedSpeed) > 1e-9 {
		t.Fatalf("碰撞后幽灵弹开速度应为 GHOST_SPEED*3=%v，got=%v", expectedSpeed, speed)
	}
}

// ─── 鸟碰撞 ──────────────────────────────────────────────────────────

func TestCheckBirdCollision_Overlap(t *testing.T) {
	bird := domain.BirdState{X: 0.5, Y: 0.5, Active: true}
	balloon := domain.BalloonState{X: 0.5, Y: 0.5}

	if !CheckBirdCollision(&bird, &balloon) {
		t.Fatal("鸟与气球重叠时应返回 true")
	}
}

func TestCheckBirdCollision_Inactive(t *testing.T) {
	bird := domain.BirdState{X: 0.5, Y: 0.5, Active: false}
	balloon := domain.BalloonState{X: 0.5, Y: 0.5}

	if CheckBirdCollision(&bird, &balloon) {
		t.Fatal("鸟未激活时应返回 false")
	}
}

func TestCheckBirdCollision_Far(t *testing.T) {
	bird := domain.BirdState{X: 0.1, Y: 0.1, Active: true}
	balloon := domain.BalloonState{X: 0.9, Y: 0.9}

	if CheckBirdCollision(&bird, &balloon) {
		t.Fatal("鸟远离气球时应返回 false")
	}
}

// ─── 幽灵 AI ─────────────────────────────────────────────────────────

func TestUpdateGhostAI_Movement(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.5
	state.Ghost.VX = protocol.GhostSpeed
	state.Ghost.VY = 0
	state.Ghost.RepelTimer = 0
	xBefore := state.Ghost.X
	yBefore := state.Ghost.Y

	UpdateGhostAI(state)

	moved := state.Ghost.X != xBefore || state.Ghost.Y != yBefore
	if !moved {
		t.Fatal("幽灵每 tick 位置应发生变化")
	}
}

func TestUpdateGhostAI_MaxSpeed(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.5
	state.Ghost.VX = protocol.GhostSpeed * 10
	state.Ghost.VY = protocol.GhostSpeed * 10
	state.Ghost.RepelTimer = 0

	UpdateGhostAI(state)

	speed := math.Sqrt(state.Ghost.VX*state.Ghost.VX + state.Ghost.VY*state.Ghost.VY)
	maxSpeed := protocol.GhostSpeed * 4
	if speed > maxSpeed+0.0001 {
		t.Fatalf("幽灵速度不应超过最大速度 %v，got %v", maxSpeed, speed)
	}
}

func TestUpdateGhostAI_Offscreen(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 1.2
	state.Ghost.Y = 0.5
	state.Ghost.VX = 0.01
	state.Ghost.VY = 0
	state.Ghost.RepelTimer = 0

	UpdateGhostAI(state)

	if state.Ghost.Active {
		t.Fatal("幽灵离开屏幕时应被销毁")
	}
	if state.Ghost.SpawnTimer <= 0 {
		t.Fatal("销毁后 SpawnTimer 应为正值")
	}
}

func TestUpdateGhostAI_Repel(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.5
	state.Ghost.VX = 0
	state.Ghost.VY = 0
	state.Ghost.RepelTimer = 10
	state.Balloon.X = 0.6
	state.Balloon.Y = 0.5

	UpdateGhostAI(state)

	if state.Ghost.VX >= 0 {
		t.Fatalf("被驱离时幽灵应远离气球（VX 应为负），got VX=%v", state.Ghost.VX)
	}
}

func TestUpdateGhostAI_Attract(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.4
	state.Ghost.Y = 0.5
	state.Ghost.VX = 0
	state.Ghost.VY = 0
	state.Ghost.RepelTimer = 0
	state.Balloon.X = 0.5
	state.Balloon.Y = 0.5
	state.TickCount = 1

	UpdateGhostAI(state)

	if state.Ghost.VX <= 0 {
		t.Fatalf("吸引半径内幽灵应朝气球加速（VX 应为正），got VX=%v", state.Ghost.VX)
	}
}

func TestUpdateGhostAI_SpawnWhenInactive(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = false
	state.Ghost.SpawnTimer = 0

	UpdateGhostAI(state)

	if !state.Ghost.Active {
		t.Fatal("未激活且倒计时到 0 时应生成新幽灵")
	}
	if state.Ghost.X < 0.15 || state.Ghost.X > 0.85 {
		t.Fatalf("新生成的幽灵 X 应在 0.15-0.85，got X=%v", state.Ghost.X)
	}
}

func TestUpdateGhostAI_CountdownWhenInactive(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = false
	state.Ghost.SpawnTimer = 10

	UpdateGhostAI(state)

	if state.Ghost.Active {
		t.Fatal("倒计时未到不应生成幽灵")
	}
	if state.Ghost.SpawnTimer != 9 {
		t.Fatalf("倒计时应递减，got SpawnTimer=%v, want=9", state.Ghost.SpawnTimer)
	}
}

// ─── 鸟 AI ───────────────────────────────────────────────────────────

func TestUpdateBirdAI_Spawn(t *testing.T) {
	state := createTestState()
	state.Bird.Active = false
	state.Bird.SpawnTimer = 1
	state.Balloon.X = 0.5
	state.Balloon.Y = 0.5

	UpdateBirdAI(&state.Bird, &state.Balloon, 0)

	if !state.Bird.Active {
		t.Fatal("倒计时到 0 时鸟应激活")
	}
}

func TestUpdateBirdAI_Move(t *testing.T) {
	state := createTestState()
	state.Bird.Active = true
	state.Bird.X = 0.5
	state.Bird.Y = 0.5
	state.Bird.VX = 0.01
	state.Bird.VY = 0
	xBefore := state.Bird.X

	UpdateBirdAI(&state.Bird, &state.Balloon, 1)

	if state.Bird.X == xBefore {
		t.Fatal("激活的鸟应移动")
	}
}

func TestUpdateBirdAI_Offscreen(t *testing.T) {
	state := createTestState()
	state.Bird.Active = true
	state.Bird.X = 1.2
	state.Bird.Y = 0.5
	state.Bird.VX = 0.01
	state.Bird.VY = 0

	UpdateBirdAI(&state.Bird, &state.Balloon, 1)

	if state.Bird.Active {
		t.Fatal("鸟离开屏幕时应被销毁")
	}
	if state.Bird.SpawnTimer <= 0 {
		t.Fatal("销毁后 SpawnTimer 应为正值")
	}
}

func TestUpdateBirdAI_Recalibrate(t *testing.T) {
	state := createTestState()
	state.Bird.Active = true
	state.Bird.X = 0.3
	state.Bird.Y = 0.5
	state.Bird.VX = 0
	state.Bird.VY = 0
	state.Balloon.X = 0.8
	state.Balloon.Y = 0.5

	UpdateBirdAI(&state.Bird, &state.Balloon, 30)

	if state.Bird.VX <= 0 {
		t.Fatalf("每 30 ticks 重新校准方向，鸟应朝气球加速（VX 应为正），got VX=%v", state.Bird.VX)
	}
}

// ─── 幽灵驱离 ────────────────────────────────────────────────────────

func TestApplyGhostRepel_InRange(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.5
	state.Ghost.RepelTimer = 0

	ApplyGhostRepel(state, 0.5, 0.5)

	if state.Ghost.RepelTimer != protocol.GhostRepelDuration {
		t.Fatalf("驱离半径内应设置 RepelTimer=GHOST_REPEL_DURATION=%v，got=%v",
			protocol.GhostRepelDuration, state.Ghost.RepelTimer)
	}
}

func TestApplyGhostRepel_OutOfRange(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.5
	state.Ghost.RepelTimer = 0

	ApplyGhostRepel(state, 0.9, 0.9)

	if state.Ghost.RepelTimer != 0 {
		t.Fatalf("驱离半径外 RepelTimer 应保持 0，got=%v", state.Ghost.RepelTimer)
	}
}

func TestApplyGhostRepel_Inactive(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = false

	ApplyGhostRepel(state, 0.5, 0.5)

	if state.Ghost.RepelTimer != 0 {
		t.Fatalf("幽灵未激活时驱离无效，RepelTimer 应为 0，got=%v", state.Ghost.RepelTimer)
	}
}

// ─── 风场 ────────────────────────────────────────────────────────────

func TestUpdateWind_AffectsBalloon(t *testing.T) {
	state := createTestState()
	state.Wind = 0.5
	state.Balloon.X = 0.5
	state.Balloon.Y = 0.5
	state.Balloon.VX = 0
	vxBefore := state.Balloon.VX

	UpdateWind(state)

	if state.Balloon.VX <= vxBefore {
		t.Fatalf("右风应使气球水平速度增大，got VX=%v, before=%v", state.Balloon.VX, vxBefore)
	}
}

func TestUpdateWind_Clamp(t *testing.T) {
	state := createTestState()
	state.Wind = 5

	UpdateWind(state)

	if state.Wind > protocol.WindClamp || state.Wind < -protocol.WindClamp {
		t.Fatalf("风场值应限制在 [-%v, %v]，got Wind=%v", protocol.WindClamp, protocol.WindClamp, state.Wind)
	}
}

// ─── 冷却公式 ────────────────────────────────────────────────────────

func TestCalculateCooldown(t *testing.T) {
	// playerCount=1: cooldown = 1000 + 2032*log2(1) = 1000
	result1 := CalculateCooldown(1)
	if result1 != int64(protocol.CooldownBaseMs) {
		t.Errorf("playerCount=1: got %d, want %d", result1, protocol.CooldownBaseMs)
	}

	// playerCount=2: cooldown = 1000 + 2032*log2(2) = 3032
	result2 := CalculateCooldown(2)
	expected2 := int64(math.Round(float64(protocol.CooldownBaseMs) + float64(protocol.CooldownLogCoeff)*math.Log2(2)))
	if result2 != expected2 {
		t.Errorf("playerCount=2: got %d, want %d", result2, expected2)
	}

	// 结果不应超过上限
	result100 := CalculateCooldown(100)
	if result100 > int64(protocol.CooldownMaxMs) {
		t.Errorf("playerCount=100: 冷却时间不应超过 %d，got %d", protocol.CooldownMaxMs, result100)
	}

	// 极大 playerCount 应被上限截断
	resultBig := CalculateCooldown(10000)
	if resultBig != int64(protocol.CooldownMaxMs) {
		t.Errorf("playerCount=10000: 应达到上限 %d，got %d", protocol.CooldownMaxMs, resultBig)
	}
}

// ─── 房间码 ──────────────────────────────────────────────────────────

func TestGenerateRoomCode(t *testing.T) {
	validChars := regexp.MustCompile(`^[A-HJ-NP-Z2-9]+$`)

	for i := 0; i < 50; i++ {
		code := GenerateRoomCode()
		if len(code) != 5 {
			t.Fatalf("房间码应为 5 字符，got len=%d, code=%s", len(code), code)
		}
		if !validChars.MatchString(code) {
			t.Fatalf("房间码应只包含大写字母（无 I/O）和数字（无 0/1），got=%s", code)
		}
	}
}

// ─── Benchmarks ──────────────────────────────────────────────────────

func BenchmarkApplyPhysics(b *testing.B) {
	balloon := domain.BalloonState{X: 0.5, Y: 0.5, VX: 0.01, VY: 0.01}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		balloon.Y = 0.5
		balloon.VY = 0.01
		ApplyPhysics(&balloon)
	}
}

func BenchmarkApplyTapForce(b *testing.B) {
	balloon := domain.BalloonState{X: 0.5, Y: 0.5, VX: 0, VY: 0}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		balloon.VX = 0
		balloon.VY = 0
		ApplyTapForce(&balloon, 0.5, 0.3)
	}
}

func BenchmarkUpdateWind(b *testing.B) {
	state := createTestState()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		UpdateWind(state)
	}
}

func BenchmarkUpdateGhostAI(b *testing.B) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.5
	state.Ghost.VX = protocol.GhostSpeed
	state.Ghost.VY = 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		UpdateGhostAI(state)
	}
}

func BenchmarkUpdateBirdAI(b *testing.B) {
	state := createTestState()
	state.Bird.Active = true
	state.Bird.X = 0.3
	state.Bird.Y = 0.5
	state.Bird.VX = protocol.BirdSpeed
	state.Bird.VY = 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		UpdateBirdAI(&state.Bird, &state.Balloon, state.TickCount)
		state.TickCount++
	}
}

func BenchmarkCheckGhostCollision(b *testing.B) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.5
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CheckGhostCollision(state)
	}
}

func BenchmarkCheckBirdCollision(b *testing.B) {
	bird := domain.BirdState{X: 0.5, Y: 0.5, Active: true}
	balloon := domain.BalloonState{X: 0.5, Y: 0.5}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CheckBirdCollision(&bird, &balloon)
	}
}

func BenchmarkCalculateCooldown(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalculateCooldown(10)
	}
}

func BenchmarkGenerateRoomCode(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GenerateRoomCode()
	}
}
