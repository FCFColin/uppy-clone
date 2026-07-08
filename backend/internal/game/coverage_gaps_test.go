package game

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/alicebob/miniredis/v2"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
	"github.com/uppy-clone/backend/internal/store"
)

func TestUpdateGhostAI_RepelZeroDistance(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = state.Balloon.X
	state.Ghost.Y = state.Balloon.Y
	state.Ghost.RepelTimer = 5
	state.Ghost.VX = 0
	state.Ghost.VY = 0
	UpdateGhostAI(state, testRNG())
}

func TestApplyTapForce_InRangeForce(t *testing.T) {
	balloon := domain.BalloonState{X: 0.5, Y: 0.5, VX: 0, VY: 0}
	if !ApplyTapForce(&balloon, 0.52, 0.48) {
		t.Fatal("expected in-range tap to apply force")
	}
	if balloon.VX == 0 && balloon.VY == 0 {
		t.Fatal("expected velocity change")
	}
}

func TestRoom_enqueueOutbound_PublishesToBroadcaster(t *testing.T) {
	bc := newMockBroadcaster()
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, bc)
	r := NewRoom("PUB1", h, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = false
	addConnectedPlayer(r, "p1")
	r.mu.Lock()
	r.enqueueOutbound([]byte("x"), broadcastOpts{excludePlayerID: "p1"})
	r.mu.Unlock()
	time.Sleep(100 * time.Millisecond)
	bc.mu.Lock()
	n := len(bc.published)
	bc.mu.Unlock()
	if n == 0 {
		t.Fatal("expected redis publish")
	}
	r.stopOutbound()
}

func TestRoom_snapshotConnTargetsLocked_IncludesValidTarget(t *testing.T) {
	r := NewRoom("TG", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.mu.Lock()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 1)}
	r.connections["p2"] = &PlayerConn{PlayerID: "p2", Send: make(chan []byte, 1)}
	targets := r.SnapshotTargets("p1")
	r.mu.Unlock()
	if len(targets) != 1 || targets[0].playerID != "p2" {
		t.Fatalf("targets = %+v", targets)
	}
}

func TestRoom_RequestPersist_UpdatesPersistLag(t *testing.T) {
	repo := newMockRoomRepository()
	r := NewRoom("LAG", nil, repo, config.DefaultTimeoutConfig(), 0)
	r.persistMu.Lock()
	r.lastPersistAt = time.Now().Add(-500 * time.Millisecond)
	r.persistMu.Unlock()
	r.mu.Lock()
	r.requestPersist()
	r.mu.Unlock()
	time.Sleep(200 * time.Millisecond)
	r.stopPersist()
}

func TestRoom_addNewPlayer_ClosesConnWhenFull(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		up := websocket.Upgrader{}
		up.Upgrade(w, req, nil)
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	r := NewRoom("FULL2", nil, nil, config.DefaultTimeoutConfig(), 1)
	r.state.Players["p0"] = &domain.PlayerState{ID: "p0", Nickname: "taken"}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 1)}

	_, err = r.addNewPlayer("p1", conn)
	if err != ErrRoomFull {
		t.Fatalf("err = %v", err)
	}
}

func TestRoom_handleAutoRestart_PruneDisconnectedVotes(t *testing.T) {
	r := NewRoom("AR4", nil, nil, config.DefaultTimeoutConfig(), 4)
	r.state.Phase = domain.PhaseEnded
	addConnectedPlayer(r, "p1")
	r.state.RestartVotes = map[string]bool{"gone": true, "p1": false}
	r.handleAutoRestart()
}

func TestHandleRestartVote_DuplicateWhenEnded(t *testing.T) {
	r := NewRoom("DUP", nil, nil, config.DefaultTimeoutConfig(), 4)
	r.syncOutbound = true
	r.mu.Lock()
	r.state.Phase = domain.PhaseEnded
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1"}
	r.state.Players["p2"] = &domain.PlayerState{ID: "p2"}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 4)}
	r.connections["p2"] = &PlayerConn{PlayerID: "p2", Send: make(chan []byte, 4)}
	r.state.RestartVotes = map[string]bool{"p1": true}
	now := time.Now().UnixMilli()
	r.state.RestartTimerStart = &now
	_ = HandleRestartVote(r, r.state.Players["p1"])
	r.mu.Unlock()
}

func TestCheckRestartConsensus_WithExistingTimer(t *testing.T) {
	r := NewRoom("RST", nil, nil, config.DefaultTimeoutConfig(), 4)
	r.syncOutbound = true
	r.mu.Lock()
	r.state.Phase = domain.PhaseEnded
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1"}
	r.state.Players["p2"] = &domain.PlayerState{ID: "p2"}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 4)}
	r.connections["p2"] = &PlayerConn{PlayerID: "p2", Send: make(chan []byte, 4)}
	now := time.Now().UnixMilli()
	r.state.RestartTimerStart = &now
	r.state.RestartVotes = map[string]bool{"p1": true}
	_ = CheckRestartConsensus(r)
	r.mu.Unlock()
}

func TestRoom_decodeTapPayload_DecodeFailure(t *testing.T) {
	r := &Room{state: NewGameState("T", testRNG()), rng: testRNG()}
	_, _, ok := r.decodeTapPayload([]byte{1, 2, 3})
	if ok {
		t.Fatal("short payload should fail decode")
	}
}

func TestRoom_handleSetNicknameMsg_EmptySanitized(t *testing.T) {
	r := NewRoom("EMP", nil, nil, config.DefaultTimeoutConfig(), 0)
	player := &domain.PlayerState{ID: "p1", Nickname: "Old"}
	// valid framing but nickname becomes empty after sanitize
	payload := append([]byte{byte(3)}, []byte("   ")...)
	r.mu.Lock()
	r.handleSetNicknameMsg(player, payload)
	r.mu.Unlock()
	if player.NicknameConfirmed {
		t.Fatal("whitespace-only nickname should not confirm")
	}
}

func TestRoom_handleSetNicknameMsg_AcceptsValidNickname(t *testing.T) {
	r := NewRoom("OK", nil, nil, config.DefaultTimeoutConfig(), 0)
	player := &domain.PlayerState{ID: "p1", Nickname: "Old"}
	payload := append([]byte{byte(len("Valid"))}, []byte("Valid")...)
	r.mu.Lock()
	r.handleSetNicknameMsg(player, payload)
	r.mu.Unlock()
	if !player.NicknameConfirmed {
		t.Fatal("valid nickname should confirm")
	}
}

type recordErrRepo struct {
	mockRoomRepository
}

func (r *recordErrRepo) RecordGameResult(_ context.Context, _, _ string, _ int64, _ int, _ []domain.GameResultPlayer) error {
	return errors.New("record failed")
}

func TestRoom_EnqueueGameResultAsync_RecordGameResultError(t *testing.T) {
	repo := &recordErrRepo{mockRoomRepository: *newMockRoomRepository()}
	r := NewRoom("RGE", nil, repo, config.DefaultTimeoutConfig(), 0)
	r.state.SessionID = "sess-1"
	r.enqueueGameResultAsync()
	r.asyncWg.Wait()
}

func TestHub_CheckRoomCached_RoomNotFound(t *testing.T) {
	h, _ := setupHubWithMiniredis(t, nil)
	info, err := h.CheckRoomCached(context.Background(), "NOPE")
	if err != nil || info != nil {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}

func TestHub_ResolveRoom_EmptyRegistryInstance(t *testing.T) {
	h, redisStore := setupHubWithMiniredis(t, nil)
	ctx := context.Background()
	data, _ := json.Marshal(domain.RoomRegistryInfo{Code: "EMPTY", Instance: "", Address: "x"})
	_ = redisStore.RegisterRoom(ctx, "EMPTY", data, time.Hour)

	decision, err := h.ResolveRoom(ctx, "EMPTY")
	if err != nil || decision.Route != RouteLocal {
		t.Fatalf("decision=%+v err=%v", decision, err)
	}
}

func TestHub_CleanupOnce_SkipsMissingRoom(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	h.mu.Lock()
	h.rooms["GHOST"] = NewRoom("GHOST", h, nil, config.DefaultTimeoutConfig(), 4)
	delete(h.rooms, "GHOST")
	h.mu.Unlock()
	h.cleanupOnce()
}

func TestHub_CreateRoom_RetryOnConflict(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	codes := []string{"AAAAA", "BBBBB"}
	i := 0
	restore := SetGenerateRoomCodeHook(func() string {
		c := codes[i]
		if i < len(codes)-1 {
			i++
		}
		return c
	})
	defer restore()

	h.mu.Lock()
	h.rooms["AAAAA"] = NewRoom("AAAAA", h, nil, config.DefaultTimeoutConfig(), 4)
	h.mu.Unlock()

	code, err := h.CreateRoom(context.Background())
	if err != nil || code != "BBBBB" {
		t.Fatalf("code=%q err=%v", code, err)
	}
}

func TestDefaultInstanceID_UnknownOnEmptyHostname(t *testing.T) {
	t.Setenv("INSTANCE_ID", "")
	prev := hostnameFn
	hostnameFn = func() (string, error) { return "", errors.New("no hostname") }
	defer func() { hostnameFn = prev }()
	if got := defaultInstanceID(); got != "unknown" {
		t.Fatalf("defaultInstanceID = %q, want unknown", got)
	}
}

func TestRoom_Close_WithPersistFlush(t *testing.T) {
	repo := newMockRoomRepository()
	r := NewRoom("CL2", nil, repo, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = true
	addConnectedPlayer(r, "p1")
	r.mu.Lock()
	r.state.Phase = domain.PhasePlaying
	r.mu.Unlock()
	r.startTick()
	r.Close()
}

func TestSaveStateWithError_SerializeError(t *testing.T) {
	repo := newMockRoomRepository()
	r := NewRoom("SER", nil, repo, config.DefaultTimeoutConfig(), 0)
	restore := SetSerializeStateHook(func(*domain.GameState) ([]byte, error) {
		return nil, errors.New("serialize failed")
	})
	defer restore()
	if err := r.saveStateWithError(); err == nil {
		t.Fatal("expected serialize error")
	}
}

func TestRoom_RequestPersist_SerializeError(t *testing.T) {
	repo := newMockRoomRepository()
	r := NewRoom("RSE", nil, repo, config.DefaultTimeoutConfig(), 0)
	restore := SetSerializeStateHook(func(*domain.GameState) ([]byte, error) {
		return nil, errors.New("serialize failed")
	})
	defer restore()
	r.mu.Lock()
	r.requestPersist()
	r.mu.Unlock()
}

func TestRoom_FlushPersistSync_SerializeError(t *testing.T) {
	repo := newMockRoomRepository()
	r := NewRoom("FSE", nil, repo, config.DefaultTimeoutConfig(), 0)
	restore := SetSerializeStateHook(func(*domain.GameState) ([]byte, error) {
		return nil, errors.New("serialize failed")
	})
	defer restore()
	r.flushPersistSync()
}

func TestRoom_snapshotConnTargetsLocked_ConnCloseCallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		up := websocket.Upgrader{}
		up.Upgrade(w, req, nil)
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	r := NewRoom("CC", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.mu.Lock()
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 1), Conn: conn}
	targets := r.SnapshotTargets("")
	r.mu.Unlock()
	if len(targets) != 1 {
		t.Fatalf("targets = %d", len(targets))
	}
	targets[0].connClose()
}

func TestHub_CleanupLoop_TickerFires(t *testing.T) {
	prev := cleanupIntervalForTest
	cleanupIntervalForTest = 20 * time.Millisecond
	defer func() { cleanupIntervalForTest = prev }()

	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	code, _ := h.CreateRoom(context.Background())
	room := h.GetRoom(code)
	room.mu.Lock()
	room.state.Phase = domain.PhaseWaiting
	room.connections = make(map[string]*PlayerConn)
	room.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	h.CleanupLoop(ctx)
}

func TestCheckRestartConsensus_PhaseNotEnded(t *testing.T) {
	r := NewRoom("PNE", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = true
	r.state.Phase = domain.PhasePlaying
	if err := CheckRestartConsensus(r); err != nil {
		t.Fatalf("CheckRestartConsensus: %v", err)
	}
}

func TestCheckRestartConsensus_UnanimousRestart(t *testing.T) {
	r := NewRoom("UNI", nil, newMockRoomRepository(), config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = true
	r.mu.Lock()
	r.state.Phase = domain.PhaseEnded
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1", Nickname: "A"}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 8)}
	r.state.RestartVotes = map[string]bool{"p1": true}
	err := CheckRestartConsensus(r)
	r.mu.Unlock()
	if err != nil {
		t.Fatalf("CheckRestartConsensus: %v", err)
	}
	if r.state.Phase != domain.PhaseCountdown {
		t.Fatalf("phase = %s, want countdown after unanimous restart", r.state.Phase)
	}
}

func TestRoom_Close_FullShutdown(t *testing.T) {
	repo := newMockRoomRepository()
	r := NewRoom("CL3", nil, repo, config.DefaultTimeoutConfig(), 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		up := websocket.Upgrader{}
		up.Upgrade(w, req, nil)
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	r.mu.Lock()
	r.endGameTimer = time.AfterFunc(time.Hour, func() {})
	r.startDelayTimer = time.AfterFunc(time.Hour, func() {})
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 4), Conn: conn}
	r.mu.Unlock()
	r.Close()
}

func TestHub_CreateRoom_AllConflicts(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	restore := SetGenerateRoomCodeHook(func() string { return "SAME1" })
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
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	code, _ := h.CreateRoom(context.Background())
	room := h.GetRoom(code)
	room.mu.Lock()
	room.state.Phase = domain.PhasePlaying
	room.mu.Unlock()
	code2, err := h.MatchRoom(context.Background())
	if err != nil || code2 == code {
		t.Fatalf("MatchRoom = %q err=%v", code2, err)
	}
}

func TestRoom_EnqueueGameResultAsync_RecordOnlyPath(t *testing.T) {
	repo := &recordErrRepo{mockRoomRepository: *newMockRoomRepository()}
	r := NewRoom("ROP", nil, repo, config.DefaultTimeoutConfig(), 0)
	r.state.SessionID = "sess-rop"
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1"}
	r.enqueueGameResultAsync()
	r.asyncWg.Wait()
}

func TestHub_CreateRoom_InvalidCodeLogged(t *testing.T) {
	restore := SetGenerateRoomCodeHook(func() string { return "AB0DE" })
	defer restore()
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	code, err := h.CreateRoom(context.Background())
	if err != nil || code != "AB0DE" {
		t.Fatalf("code=%q err=%v", code, err)
	}
}

func TestUpdateWind_WindClampAfterLerp(t *testing.T) {
	state := createTestState()
	state.Wind = 5
	state.WindTarget = 5
	state.WindMidOffset = 0
	state.WindMicroCountdown = 100
	state.WindMidCountdown = 100
	state.WindChangeCountdown = 100
	state.Balloon.X = 0.5
	UpdateWind(state, testRNG())
	if state.Wind != protocol.WindClamp {
		t.Fatalf("Wind = %v, want clamp %v", state.Wind, protocol.WindClamp)
	}
}

func TestCheckRestartConsensus_NotEndedAfterBroadcast(t *testing.T) {
	r := NewRoom("NBA", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = true
	r.state.Phase = domain.PhaseCountdown
	if err := CheckRestartConsensus(r); err != nil {
		t.Fatalf("CheckRestartConsensus: %v", err)
	}
}

func TestRoom_EnqueueGameResultAsync_FullRedisPath(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	redisStore, err := store.NewRedisStore(mr.Addr(), config.DefaultTimeoutConfig())
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	defer redisStore.Close()
	repo := newMockRoomRepository()
	h := NewHub(repo, redisStore, config.DefaultTimeoutConfig(), 0, 0, nil)
	r := NewRoom("FRP", h, repo, config.DefaultTimeoutConfig(), 0)
	r.state.SessionID = "sess-frp"
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1", ScoreContribution: 1, TapsCount: 1}
	r.enqueueGameResultAsync()
	r.asyncWg.Wait()
}

func TestHub_cleanupOnce_KeepsPlayingRoom(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	code, _ := h.CreateRoom(context.Background())
	room := h.GetRoom(code)
	addConnectedPlayer(room, "p1")
	room.mu.Lock()
	room.state.Phase = domain.PhasePlaying
	room.mu.Unlock()
	h.cleanupOnce()
	if h.GetRoom(code) == nil {
		t.Fatal("playing room with connections should not be cleaned")
	}
}

func TestUpdateWind_NegativeWindClampAfterLerp(t *testing.T) {
	state := createTestState()
	state.Wind = -5
	state.WindTarget = -5
	state.WindMidOffset = 0
	state.WindMicroCountdown = 100
	state.WindMidCountdown = 100
	state.WindChangeCountdown = 100
	state.Balloon.X = 0.5
	UpdateWind(state, testRNG())
	if state.Wind != -protocol.WindClamp {
		t.Fatalf("Wind = %v, want clamp %v", state.Wind, -protocol.WindClamp)
	}
}

func TestUpdateBirdAI_RecalibrateZeroDistance(t *testing.T) {
	state := createTestState()
	state.Bird.Active = true
	state.Bird.X = state.Balloon.X
	state.Bird.Y = state.Balloon.Y
	state.Bird.VX = 0.01
	UpdateBirdAI(&state.Bird, &state.Balloon, 30, testRNG())
}

func TestCheckRestartConsensus_NegativeRemainingTimer(t *testing.T) {
	r := NewRoom("NEG", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.syncOutbound = true
	r.mu.Lock()
	r.state.Phase = domain.PhaseEnded
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1"}
	r.state.Players["p2"] = &domain.PlayerState{ID: "p2"}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 4)}
	r.connections["p2"] = &PlayerConn{PlayerID: "p2", Send: make(chan []byte, 4)}
	past := time.Now().UnixMilli() - int64(domain.RestartTimeoutMs) - 1000
	r.state.RestartTimerStart = &past
	r.state.RestartVotes = map[string]bool{"p1": true}
	_ = CheckRestartConsensus(r)
	r.mu.Unlock()
}

func TestHub_cleanupOnce_RemovesAllDisconnectedExpired(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	code, _ := h.CreateRoom(context.Background())
	room := h.GetRoom(code)
	now := time.Now().UnixMilli()
	expired := now - domain.ReconnectGraceMs - 1000
	room.mu.Lock()
	room.state.Phase = domain.PhaseWaiting
	room.state.Players["p1"] = &domain.PlayerState{
		ID: "p1", Nickname: "gone", Disconnected: true, DisconnectedAt: &expired,
	}
	room.connections = make(map[string]*PlayerConn)
	room.mu.Unlock()
	h.cleanupOnce()
	if h.GetRoom(code) != nil {
		t.Fatal("expected room with all expired disconnected players to be cleaned")
	}
}

func TestHub_cleanupOnce_SkipsMissingRoom(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	prev := snapshotRoomCodesHook
	snapshotRoomCodesHook = func(*Hub) []string { return []string{"MISSING1"} }
	t.Cleanup(func() { snapshotRoomCodesHook = prev })
	h.cleanupOnce()
}

func TestUpdateWind_AllCountdownResets(t *testing.T) {
	state := createTestState()
	state.WindMicroCountdown = 1
	state.WindMidCountdown = 1
	state.WindChangeCountdown = 1
	state.Wind = 0
	state.WindTarget = 0
	state.WindMidOffset = 0
	state.Balloon.X = 0.5
	UpdateWind(state, testRNG())
	if state.WindMicroCountdown != protocol.WindMicroInterval {
		t.Fatalf("WindMicroCountdown = %d", state.WindMicroCountdown)
	}
}

func TestUpdateWind_RightEdgeSoftZone(t *testing.T) {
	center := createTestState()
	center.Balloon.X = 0.5
	center.Wind = 2
	center.WindTarget = 2
	center.WindMidOffset = 0
	center.WindMicroCountdown = 100
	center.WindMidCountdown = 100
	center.WindChangeCountdown = 100
	centerVX := center.Balloon.VX
	UpdateWind(center, testRNG())
	centerDelta := center.Balloon.VX - centerVX

	right := createTestState()
	right.Balloon.X = 1 - protocol.WindEdgeSoftZone/2
	right.Wind = 2
	right.WindTarget = 2
	right.WindMidOffset = 0
	right.WindMicroCountdown = 100
	right.WindMidCountdown = 100
	right.WindChangeCountdown = 100
	rightVX := right.Balloon.VX
	UpdateWind(right, testRNG())
	rightDelta := right.Balloon.VX - rightVX

	if rightDelta >= centerDelta {
		t.Fatalf("right edge delta=%v should be less than center delta=%v", rightDelta, centerDelta)
	}
}

func TestHub_cleanupOnce_KeepsDisconnectedWithoutTimestamp(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	code, _ := h.CreateRoom(context.Background())
	room := h.GetRoom(code)
	room.mu.Lock()
	room.state.Phase = domain.PhasePlaying
	room.state.Players["p1"] = &domain.PlayerState{ID: "p1", Disconnected: true, DisconnectedAt: nil}
	room.connections = make(map[string]*PlayerConn)
	room.mu.Unlock()
	h.cleanupOnce()
	if h.GetRoom(code) == nil {
		t.Fatal("room with disconnected player without timestamp should not be cleaned")
	}
}

func TestRoom_EnqueueGameResultAsync_RedisOnlyNoStore(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	redisStore, err := store.NewRedisStore(mr.Addr(), config.DefaultTimeoutConfig())
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	defer redisStore.Close()
	h := NewHub(nil, redisStore, config.DefaultTimeoutConfig(), 0, 0, nil)
	r := NewRoom("RNS", h, nil, config.DefaultTimeoutConfig(), 0)
	r.state.SessionID = "sess-rns"
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1"}
	r.enqueueGameResultAsync()
	r.asyncWg.Wait()
}

func TestRoom_EnqueueGameResultAsync_MarshalPayloadError(t *testing.T) {
	prev := jsonMarshalGameResultFn
	jsonMarshalGameResultFn = func(any) ([]byte, error) {
		return nil, errors.New("marshal failed")
	}
	t.Cleanup(func() { jsonMarshalGameResultFn = prev })

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	redisStore, err := store.NewRedisStore(mr.Addr(), config.DefaultTimeoutConfig())
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	defer redisStore.Close()
	h := NewHub(nil, redisStore, config.DefaultTimeoutConfig(), 0, 0, nil)
	r := NewRoom("MRJ", h, newMockRoomRepository(), config.DefaultTimeoutConfig(), 0)
	r.state.SessionID = "sess-mrj"
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1"}
	r.enqueueGameResultAsync()
	r.asyncWg.Wait()
}

func TestRoom_EnqueueGameResultAsync_OutboxPayloadError(t *testing.T) {
	prev := gameEndedOutboxPayloadFn
	gameEndedOutboxPayloadFn = func(map[string]interface{}) ([]byte, error) {
		return nil, errors.New("outbox failed")
	}
	t.Cleanup(func() { gameEndedOutboxPayloadFn = prev })

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	redisStore, err := store.NewRedisStore(mr.Addr(), config.DefaultTimeoutConfig())
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	defer redisStore.Close()
	repo := newMockRoomRepository()
	h := NewHub(repo, redisStore, config.DefaultTimeoutConfig(), 0, 0, nil)
	r := NewRoom("OEJ", h, repo, config.DefaultTimeoutConfig(), 0)
	r.state.SessionID = "sess-oej"
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1"}
	r.enqueueGameResultAsync()
	r.asyncWg.Wait()
}
