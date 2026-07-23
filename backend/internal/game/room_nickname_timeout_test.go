package game

import (
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
)

// withShortNicknameTimeout 临时将昵称确认超时设为短时间，返回恢复函数。
// 调用方应在 defer 中执行返回的函数以恢复原始值。
func withShortNicknameTimeout(d time.Duration) func() {
	orig := nicknameConfirmTimeout
	nicknameConfirmTimeout = d
	return func() { nicknameConfirmTimeout = orig }
}

// waitForPlayerRemoved 轮询检查玩家是否已从房间移除，最多等待 timeout。
func waitForPlayerRemoved(t *testing.T, r *Room, playerID string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		r.mu.RLock()
		_, exists := r.state.Players[playerID]
		r.mu.RUnlock()
		if !exists {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// TestRoom_NicknameTimeout_KicksPlayer 验证玩家加入后超时未确认昵称会被自动踢出。
func TestRoom_NicknameTimeout_KicksPlayer(t *testing.T) {
	restore := withShortNicknameTimeout(100 * time.Millisecond)
	defer restore()

	r := NewRoom("NT1", nil, nil, config.DefaultTimeoutConfig(), 0)
	defer r.Close()

	addConnectedPlayer(r, "p1")

	r.mu.Lock()
	r.startNicknameTimer("p1")
	r.mu.Unlock()

	if !waitForPlayerRemoved(t, r, "p1", 2*time.Second) {
		t.Fatal("expected player to be kicked after nickname timeout")
	}

	// 定时器应在踢出后被清理
	r.mu.RLock()
	_, timerExists := r.nicknameTimers["p1"]
	connCount := len(r.connections)
	r.mu.RUnlock()

	if timerExists {
		t.Fatal("expected nickname timer to be cleared after kick")
	}
	if connCount != 0 {
		t.Fatalf("expected 0 connections after kick, got %d", connCount)
	}
}

// TestRoom_NicknameTimeout_CancelledOnConfirm 验证玩家在超时前确认昵称，
// 定时器被取消，玩家不被踢出。
func TestRoom_NicknameTimeout_CancelledOnConfirm(t *testing.T) {
	restore := withShortNicknameTimeout(500 * time.Millisecond)
	defer restore()

	r := NewRoom("NT2", nil, nil, config.DefaultTimeoutConfig(), 0)
	defer r.Close()

	addConnectedPlayer(r, "p1")

	r.mu.Lock()
	r.startNicknameTimer("p1")
	r.mu.Unlock()

	// 在超时前确认昵称（使用与当前昵称相同的名字触发"同名确认"分支）
	currentNick := "Playerp1"
	payload := append([]byte{byte(len(currentNick))}, []byte(currentNick)...)
	r.mu.Lock()
	player := r.state.Players["p1"]
	r.handleSetNicknameMsg(player, payload)
	r.mu.Unlock()

	if !player.NicknameConfirmed {
		t.Fatal("player should have nickname confirmed")
	}

	// 确认定时器已被取消
	r.mu.RLock()
	_, timerExists := r.nicknameTimers["p1"]
	r.mu.RUnlock()
	if timerExists {
		t.Fatal("nickname timer should be cleared after confirm")
	}

	// 等待超过原超时时间，确认玩家未被踢出
	time.Sleep(700 * time.Millisecond)

	r.mu.RLock()
	_, exists := r.state.Players["p1"]
	r.mu.RUnlock()
	if !exists {
		t.Fatal("player should not be kicked after confirming nickname")
	}
}

// TestRoom_NicknameTimeout_CancelledOnConfirm_NewName 验证通过设置新昵称确认时
// 定时器也被取消。
func TestRoom_NicknameTimeout_CancelledOnConfirm_NewName(t *testing.T) {
	restore := withShortNicknameTimeout(500 * time.Millisecond)
	defer restore()

	r := NewRoom("NT2B", nil, nil, config.DefaultTimeoutConfig(), 0)
	defer r.Close()

	addConnectedPlayer(r, "p1")

	r.mu.Lock()
	r.startNicknameTimer("p1")
	r.mu.Unlock()

	// 使用与当前昵称不同的新名字触发 HandleSetNickname 分支
	newNick := "Alice"
	payload := append([]byte{byte(len(newNick))}, []byte(newNick)...)
	r.mu.Lock()
	player := r.state.Players["p1"]
	r.handleSetNicknameMsg(player, payload)
	r.mu.Unlock()

	if !player.NicknameConfirmed {
		t.Fatal("player should have nickname confirmed")
	}

	r.mu.RLock()
	_, timerExists := r.nicknameTimers["p1"]
	r.mu.RUnlock()
	if timerExists {
		t.Fatal("nickname timer should be cleared after confirm with new name")
	}

	// 等待超过原超时时间，确认玩家未被踢出
	time.Sleep(700 * time.Millisecond)

	r.mu.RLock()
	_, exists := r.state.Players["p1"]
	r.mu.RUnlock()
	if !exists {
		t.Fatal("player should not be kicked after confirming nickname")
	}
}

// TestRoom_NicknameTimeout_DisconnectCancelsTimer 验证玩家断连后定时器被取消，
// 不会在断连期间触发踢出。
func TestRoom_NicknameTimeout_DisconnectCancelsTimer(t *testing.T) {
	restore := withShortNicknameTimeout(200 * time.Millisecond)
	defer restore()

	r := NewRoom("NT3", nil, nil, config.DefaultTimeoutConfig(), 0)
	defer r.Close()

	addConnectedPlayer(r, "p1")

	r.mu.Lock()
	r.startNicknameTimer("p1")
	r.mu.Unlock()

	// 玩家断连 → 定时器应被取消
	_ = r.HandleDisconnect("p1")

	r.mu.RLock()
	_, timerExists := r.nicknameTimers["p1"]
	r.mu.RUnlock()
	if timerExists {
		t.Fatal("nickname timer should be cleared on disconnect")
	}

	// 等待超过原超时时间，玩家应仍存在于 state.Players（标记为 Disconnected）
	time.Sleep(400 * time.Millisecond)

	r.mu.RLock()
	player, exists := r.state.Players["p1"]
	r.mu.RUnlock()
	if !exists {
		t.Fatal("player should still exist after disconnect (grace period), not kicked by timer")
	}
	if player != nil && !player.Disconnected {
		t.Fatal("player should be marked as disconnected")
	}
}

// TestRoom_NicknameTimeout_ReconnectRestartsTimer 验证玩家断连后重连
// （NicknameConfirmed=false）会重新启动超时定时器，且超时后会被踢出。
func TestRoom_NicknameTimeout_ReconnectRestartsTimer(t *testing.T) {
	restore := withShortNicknameTimeout(100 * time.Millisecond)
	defer restore()

	r := NewRoom("NT4", nil, nil, config.DefaultTimeoutConfig(), 0)
	defer r.Close()

	// 添加一个已断连、未确认昵称的玩家
	now := time.Now().UnixMilli()
	r.mu.Lock()
	r.state.Players["p1"] = &domain.PlayerState{
		ID:                "p1",
		PlayerIndex:       0,
		Nickname:          "Playerp1",
		Disconnected:      true,
		DisconnectedAt:    &now,
		NicknameConfirmed: false,
	}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 64)}
	r.usedNames["Playerp1"] = true
	r.mu.Unlock()

	// 重连 → 应启动超时定时器
	r.mu.Lock()
	r.reconnectPlayer("p1", r.state.Players["p1"])
	_, timerStarted := r.nicknameTimers["p1"]
	r.mu.Unlock()

	if !timerStarted {
		t.Fatal("expected nickname timer to be started on reconnect")
	}

	// 等待超时触发踢出
	if !waitForPlayerRemoved(t, r, "p1", 2*time.Second) {
		t.Fatal("expected player to be kicked after reconnect nickname timeout")
	}
}

// TestRoom_NicknameTimeout_CloseClearsTimers 验证 Room.Close() 后所有定时器被停止。
func TestRoom_NicknameTimeout_CloseClearsTimers(t *testing.T) {
	// 使用长超时，确保 Close 在超时前执行
	restore := withShortNicknameTimeout(10 * time.Second)
	defer restore()

	r := NewRoom("NT5", nil, nil, config.DefaultTimeoutConfig(), 0)

	addConnectedPlayer(r, "p1")
	addConnectedPlayer(r, "p2")

	r.mu.Lock()
	r.startNicknameTimer("p1")
	r.startNicknameTimer("p2")
	timerCount := len(r.nicknameTimers)
	r.mu.Unlock()

	if timerCount != 2 {
		t.Fatalf("expected 2 timers before Close, got %d", timerCount)
	}

	r.Close()

	r.mu.RLock()
	remaining := len(r.nicknameTimers)
	r.mu.RUnlock()

	if remaining != 0 {
		t.Fatalf("expected 0 timers after Close, got %d", remaining)
	}
}

// TestRoom_NicknameTimeout_ConfirmedPlayerNotKicked 验证已确认昵称的玩家
// 不会被超时机制踢出（即使手动启动了定时器，回调中也会跳过）。
func TestRoom_NicknameTimeout_ConfirmedPlayerNotKicked(t *testing.T) {
	restore := withShortNicknameTimeout(100 * time.Millisecond)
	defer restore()

	r := NewRoom("NT6", nil, nil, config.DefaultTimeoutConfig(), 0)
	defer r.Close()

	// 添加一个已确认昵称的玩家
	r.mu.Lock()
	r.state.Players["p1"] = &domain.PlayerState{
		ID:                "p1",
		PlayerIndex:       0,
		Nickname:          "Playerp1",
		NicknameConfirmed: true,
	}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 64)}
	r.usedNames["Playerp1"] = true
	r.startNicknameTimer("p1")
	r.mu.Unlock()

	// 等待超过超时时间
	time.Sleep(400 * time.Millisecond)

	r.mu.RLock()
	_, exists := r.state.Players["p1"]
	r.mu.RUnlock()

	if !exists {
		t.Fatal("confirmed player should not be kicked by nickname timeout")
	}
}
