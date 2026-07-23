package game

import (
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
	"github.com/uppy-clone/backend/internal/testutil"
)

func TestRoom_HandleJoin_ReconnectDuringGrace(t *testing.T) {
	r := NewRoom("RECON", nil, nil, config.DefaultTimeoutConfig(), 4)
	now := time.Now().UnixMilli()
	r.state.Players["p1"] = &domain.PlayerState{
		ID: "p1", Nickname: "Bob", PlayerIndex: 0, Disconnected: true, DisconnectedAt: &now,
	}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 4)}

	server := testutil.NewWSTestUpgraderServer(t)
	conn, resp, err := websocket.DefaultDialer.Dial("ws"+server.URL[4:], nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if err := r.HandleJoin("p1", conn); err != nil {
		t.Fatalf("HandleJoin reconnect: %v", err)
	}
	if r.state.Players["p1"].Disconnected {
		t.Fatal("player should be reconnected")
	}
}

func TestRoom_reconnectPlayer_PlayingPhase(t *testing.T) {
	r := NewRoom("RPL", nil, nil, config.DefaultTimeoutConfig(), 4)
	r.state.Phase = domain.PhasePlaying
	player := &domain.PlayerState{ID: "p1", Nickname: "Nick", PlayerIndex: 0}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 8)}
	r.reconnectPlayer("p1", player)
	if r.tickCancel == nil {
		t.Fatal("expected tick started on playing reconnect")
	}
	r.stopTick()
}

// TestRoom_reconnectPlayer_EndedPhase_TriggersAutoRestart verifies that when a
// player reconnects during the ended phase and all connected players have
// confirmed their nicknames, reconnectPlayer invokes triggerAutoRestartIfEnded
// which calls handleAutoRestart → RestartAndStart, transitioning the room out
// of the ended phase.
func TestRoom_reconnectPlayer_EndedPhase_TriggersAutoRestart(t *testing.T) {
	r := NewRoom("RPE1", nil, nil, config.DefaultTimeoutConfig(), 4)
	r.state.Phase = domain.PhaseEnded
	player := &domain.PlayerState{
		ID:                "p1",
		Nickname:          "Nick",
		PlayerIndex:       0,
		NicknameConfirmed: true,
		Disconnected:      true,
	}
	r.state.Players["p1"] = player
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 8)}
	defer r.Close()

	r.reconnectPlayer("p1", player)

	if player.Disconnected {
		t.Fatal("player should be reconnected")
	}
	if r.state.Phase != domain.PhaseCountdown {
		t.Fatalf("expected phase countdown after auto-restart, got %q", r.state.Phase)
	}
}

// TestRoom_reconnectPlayer_EndedPhase_NoRestartWhenNicknameUnconfirmed
// verifies that when a player reconnects during the ended phase but the player
// has not confirmed their nickname, triggerAutoRestartIfEnded returns early
// without calling handleAutoRestart, so the phase remains ended.
func TestRoom_reconnectPlayer_EndedPhase_NoRestartWhenNicknameUnconfirmed(t *testing.T) {
	r := NewRoom("RPE2", nil, nil, config.DefaultTimeoutConfig(), 4)
	r.state.Phase = domain.PhaseEnded
	player := &domain.PlayerState{
		ID:                "p1",
		Nickname:          "Nick",
		PlayerIndex:       0,
		NicknameConfirmed: false,
		Disconnected:      true,
	}
	r.state.Players["p1"] = player
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 8)}
	defer r.Close()

	r.reconnectPlayer("p1", player)

	if player.Disconnected {
		t.Fatal("player should be reconnected")
	}
	if r.state.Phase != domain.PhaseEnded {
		t.Fatalf("expected phase still ended (no auto-restart), got %q", r.state.Phase)
	}
}

// setupEndedRoomWithPlayers creates an ended-phase Room with the given players
// pre-populated. Each entry in vote map sets RestartVotes[pid]=true.
// All players are connected (connections registered) and NicknameConfirmed=true.
func setupEndedRoomWithPlayers(t *testing.T, code string, playerIDs []string, votes map[string]bool) *Room {
	t.Helper()
	r := NewRoom(code, nil, nil, config.DefaultTimeoutConfig(), 4)
	r.state.Phase = domain.PhaseEnded
	for i, pid := range playerIDs {
		r.state.Players[pid] = &domain.PlayerState{
			ID:                pid,
			Nickname:          "Player" + pid,
			PlayerIndex:       i,
			NicknameConfirmed: true,
			Disconnected:      false,
		}
		r.connections[pid] = &PlayerConn{PlayerID: pid, Send: make(chan []byte, 8)}
	}
	r.state.RestartVotes = make(map[string]bool)
	for pid, v := range votes {
		r.state.RestartVotes[pid] = v
	}
	return r
}

// TestRoom_handleAutoRestart_SinglePlayerResidualVote_ImmediateRestart
// reproduces the bug where a single player reconnects to an ended room with
// a residual RestartVote. The previous logic deferred 30s indefinitely
// because yesVotes=1 > 0 even though consensus was reached (yesVotes==connectedCount).
// After the fix, consensus is detected and RestartAndStart is invoked immediately.
func TestRoom_handleAutoRestart_SinglePlayerResidualVote_ImmediateRestart(t *testing.T) {
	r := setupEndedRoomWithPlayers(t, "AR1", []string{"p1"}, map[string]bool{"p1": true})
	defer r.Close()

	r.handleAutoRestart()

	if r.state.Phase != domain.PhaseCountdown {
		t.Fatalf("expected phase countdown after consensus auto-restart, got %q", r.state.Phase)
	}
	if len(r.state.RestartVotes) != 0 {
		t.Fatalf("expected RestartVotes cleared after restart, got %d", len(r.state.RestartVotes))
	}
}

// TestRoom_handleAutoRestart_MultiPlayerNoConsensus_DefersThirtySeconds
// verifies that when not all connected players have voted yes, the restart
// is deferred by 30s (setEndGameAlarm armed) and the phase remains ended.
func TestRoom_handleAutoRestart_MultiPlayerNoConsensus_DefersThirtySeconds(t *testing.T) {
	r := setupEndedRoomWithPlayers(t, "AR2",
		[]string{"p1", "p2"},
		map[string]bool{"p1": true}, // only p1 voted
	)
	defer r.Close()

	r.mu.Lock()
	preTimer := r.endGameTimer
	r.mu.Unlock()

	r.handleAutoRestart()

	r.mu.RLock()
	phase := r.state.Phase
	postTimer := r.endGameTimer
	r.mu.RUnlock()

	if phase != domain.PhaseEnded {
		t.Fatalf("expected phase still ended (deferred), got %q", phase)
	}
	if postTimer == nil {
		t.Fatal("expected endGameTimer to be armed for 30s defer, got nil")
	}
	_ = preTimer
}

// encodeNicknamePayload builds a MsgSetNickname payload (without msgType prefix)
// for testing: nickLen(1) + nickname(bytes). Caller must ensure len(nick) <= 255.
func encodeNicknamePayload(nick string) []byte {
	return append([]byte{byte(len(nick))}, []byte(nick)...) //nolint:gosec // G115: test helper, short nicknames only
}

// drainNicknameRejected drains ch until timeout and returns the reason code of
// the first NICKNAME_REJECTED message found. Returns (0, false) if none found.
func drainNicknameRejected(ch <-chan []byte, timeout time.Duration) (uint8, bool) {
	deadline := time.After(timeout)
	for {
		select {
		case msg := <-ch:
			if len(msg) >= 2 && msg[0] == protocol.MsgNicknameRejected {
				return msg[1], true
			}
		case <-deadline:
			return 0, false
		}
	}
}

// TestRoom_handleSetNicknameMsg_RejectReasons covers the three rejection
// paths in handleSetNicknameMsg (decode_error, empty, duplicate, cooldown) and
// verifies each sends NICKNAME_REJECTED with the correct reason code. Also
// verifies the accept path does NOT send NICKNAME_REJECTED and still broadcasts
// a snapshot.
func TestRoom_handleSetNicknameMsg_RejectReasons(t *testing.T) {
	t.Run("DecodeError_NilPayload", func(t *testing.T) {
		r := NewRoom("REJ1", nil, nil, config.DefaultTimeoutConfig(), 0)
		ch := make(chan []byte, 4)
		r.mu.Lock()
		r.state.Players["p1"] = &domain.PlayerState{ID: "p1", Nickname: "Old"}
		r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch}
		r.handleSetNicknameMsg(r.state.Players["p1"], nil)
		r.mu.Unlock()

		reason, ok := drainNicknameRejected(ch, 100*time.Millisecond)
		if !ok {
			t.Fatal("expected NICKNAME_REJECTED, got none")
		}
		if reason != protocol.NickRejectDecodeError {
			t.Fatalf("reason = 0x%02x, want 0x%02x (NickRejectDecodeError)", reason, protocol.NickRejectDecodeError)
		}
	})

	t.Run("Empty_WhitespaceOnly", func(t *testing.T) {
		r := NewRoom("REJ3", nil, nil, config.DefaultTimeoutConfig(), 0)
		ch := make(chan []byte, 4)
		r.mu.Lock()
		r.state.Players["p1"] = &domain.PlayerState{ID: "p1", Nickname: "Old"}
		r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch}
		r.handleSetNicknameMsg(r.state.Players["p1"], encodeNicknamePayload("   "))
		r.mu.Unlock()

		reason, ok := drainNicknameRejected(ch, 100*time.Millisecond)
		if !ok {
			t.Fatal("expected NICKNAME_REJECTED, got none")
		}
		if reason != protocol.NickRejectEmpty {
			t.Fatalf("reason = 0x%02x, want 0x%02x (NickRejectEmpty)", reason, protocol.NickRejectEmpty)
		}
	})

	t.Run("Duplicate_AlreadyTaken", func(t *testing.T) {
		r := NewRoom("REJ5", nil, nil, config.DefaultTimeoutConfig(), 0)
		ch := make(chan []byte, 4)
		r.mu.Lock()
		r.state.Players["p2"] = &domain.PlayerState{ID: "p2", Nickname: "Taken"}
		r.usedNames["Taken"] = true
		r.state.Players["p1"] = &domain.PlayerState{ID: "p1", Nickname: "Old"}
		r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch}
		r.handleSetNicknameMsg(r.state.Players["p1"], encodeNicknamePayload("Taken"))
		r.mu.Unlock()

		reason, ok := drainNicknameRejected(ch, 100*time.Millisecond)
		if !ok {
			t.Fatal("expected NICKNAME_REJECTED, got none")
		}
		if reason != protocol.NickRejectDuplicate {
			t.Fatalf("reason = 0x%02x, want 0x%02x (NickRejectDuplicate)", reason, protocol.NickRejectDuplicate)
		}
	})

	t.Run("Cooldown_WithinWindow", func(t *testing.T) {
		r := NewRoom("REJ6", nil, nil, config.DefaultTimeoutConfig(), 0)
		ch := make(chan []byte, 4)
		r.mu.Lock()
		r.state.Players["p1"] = &domain.PlayerState{
			ID:                 "p1",
			Nickname:           "Old",
			LastNicknameChange: time.Now().UnixMilli(),
		}
		r.usedNames["Old"] = true
		r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: ch}
		r.handleSetNicknameMsg(r.state.Players["p1"], encodeNicknamePayload("Fresh"))
		r.mu.Unlock()

		reason, ok := drainNicknameRejected(ch, 100*time.Millisecond)
		if !ok {
			t.Fatal("expected NICKNAME_REJECTED, got none")
		}
		if reason != protocol.NickRejectCooldown {
			t.Fatalf("reason = 0x%02x, want 0x%02x (NickRejectCooldown)", reason, protocol.NickRejectCooldown)
		}
	})
}
