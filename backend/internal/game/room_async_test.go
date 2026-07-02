package game

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/store"
)

type trackingRoomRepo struct {
	mockRoomRepository
	insertOutboxErr    error
	createSessionErr   error
	insertOutboxCount  int
	createSessionCount int
}

func (t *trackingRoomRepo) InsertOutboxEvent(_ context.Context, _, _ string, _ []byte) error {
	t.insertOutboxCount++
	return t.insertOutboxErr
}

func (t *trackingRoomRepo) CreateGameSession(_ context.Context, _ *domain.GameSession) error {
	t.createSessionCount++
	return t.createSessionErr
}

func TestRoom_EnqueueGameResultAsync_NoHub(t *testing.T) {
	r := &Room{state: NewGameState("TEST"), logger: slog.New(slog.NewTextHandler(os.Stderr, nil))}
	r.state.SessionID = "sess-1"
	r.enqueueGameResultAsync()
}

func TestRoom_EnqueueGameResultAsync_NoSessionID(t *testing.T) {
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	r := NewRoom("TEST1", h, nil, config.DefaultTimeoutConfig(), 0)
	r.enqueueGameResultAsync()
}

func TestRoom_RunGameResultJob_Success(t *testing.T) {
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

	repo := &trackingRoomRepo{mockRoomRepository: *newMockRoomRepository()}
	h := NewHub(repo, redisStore, config.DefaultTimeoutConfig(), 0, 0, nil)
	r := NewRoom("TEST1", h, repo, config.DefaultTimeoutConfig(), 0)
	r.state.SessionID = "sess-abc"
	r.state.LobbyCode = "TEST1"
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1", ScoreContribution: 10, TapsCount: 2}

	r.enqueueGameResultAsync()
	r.asyncWg.Wait()

	if repo.insertOutboxCount != 1 {
		t.Fatalf("insertOutboxCount = %d, want 1", repo.insertOutboxCount)
	}
}

func TestRoom_RunGameResultJob_OutboxError(t *testing.T) {
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

	repo := &trackingRoomRepo{
		mockRoomRepository: *newMockRoomRepository(),
		insertOutboxErr:    errors.New("outbox down"),
	}
	h := NewHub(repo, redisStore, config.DefaultTimeoutConfig(), 0, 0, nil)
	r := NewRoom("TEST1", h, repo, config.DefaultTimeoutConfig(), 0)
	r.state.SessionID = "sess-abc"
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1"}

	r.runGameResultJob(gameResultJob{
		sessionID: "sess-abc",
		roomCode:  "TEST1",
		payload:   []byte(`{"game_id":"sess-abc"}`),
		outbox:    []byte(`{"event":"game.ended"}`),
	})
}

func TestRoom_CreateGameSessionAsync(t *testing.T) {
	repo := &trackingRoomRepo{mockRoomRepository: *newMockRoomRepository()}
	r := NewRoom("TEST1", nil, repo, config.DefaultTimeoutConfig(), 0)

	r.createGameSessionAsync(&domain.GameSession{ID: "sess-1", LobbyCode: "TEST1"})
	r.asyncWg.Wait()

	if repo.createSessionCount != 1 {
		t.Fatalf("createSessionCount = %d, want 1", repo.createSessionCount)
	}
}

func TestRoom_CreateGameSessionAsync_NilSession(t *testing.T) {
	repo := &trackingRoomRepo{mockRoomRepository: *newMockRoomRepository()}
	r := NewRoom("TEST1", nil, repo, config.DefaultTimeoutConfig(), 0)
	r.createGameSessionAsync(nil)
	if repo.createSessionCount != 0 {
		t.Fatal("nil session should not call store")
	}
}

func TestRoom_FlushPersistSync(t *testing.T) {
	repo := newMockRoomRepository()
	r := NewRoom("TEST1", nil, repo, config.DefaultTimeoutConfig(), 0)
	r.state.LobbyCode = "TEST1"

	r.mu.Lock()
	r.requestPersist()
	r.mu.Unlock()
	time.Sleep(200 * time.Millisecond)

	r.flushPersistSync()
	if repo.saveCount < 1 {
		t.Fatalf("saveCount = %d, want at least 1", repo.saveCount)
	}
}

func TestRoom_RunPersistLoop_FinalJob(t *testing.T) {
	repo := newMockRoomRepository()
	r := NewRoom("FINAL", nil, repo, config.DefaultTimeoutConfig(), 0)
	r.state.LobbyCode = "FINAL"

	r.startPersistLoop()
	done := make(chan struct{})
	r.persistCh <- persistJob{
		code:      "FINAL",
		stateJSON: []byte(`{"phase":"waiting"}`),
		final:     true,
		done:      done,
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("final persist did not complete")
	}
	r.stopPersist()
}

func TestRoom_WritePersistJob_NilStore(t *testing.T) {
	r := &Room{logger: slog.New(slog.NewTextHandler(os.Stderr, nil))}
	done := make(chan struct{})
	r.writePersistJob(persistJob{done: done})
	select {
	case <-done:
	default:
		t.Fatal("done channel should close even with nil store")
	}
}

func TestRoom_WritePersistJob_StoreError(t *testing.T) {
	repo := newMockRoomRepository()
	repo.saveErr = errors.New("save failed")
	r := NewRoom("ERR", nil, repo, config.DefaultTimeoutConfig(), 0)
	r.writePersistJob(persistJob{code: "ERR", stateJSON: []byte("{}")})
}

func TestRoom_RequestPersist_NilStore(t *testing.T) {
	r := NewRoom("TEST1", nil, nil, config.DefaultTimeoutConfig(), 0)
	r.mu.Lock()
	r.requestPersist()
	r.mu.Unlock()
}

func TestRoom_RequestPersist_QueueCoalesce(t *testing.T) {
	repo := newMockRoomRepository()
	r := NewRoom("TEST1", nil, repo, config.DefaultTimeoutConfig(), 0)
	r.startPersistLoop()

	var wg sync.WaitGroup
	for i := 0; i < persistQueueSize+2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.mu.Lock()
			r.requestPersist()
			r.mu.Unlock()
		}()
	}
	wg.Wait()
	time.Sleep(200 * time.Millisecond)
	r.stopPersist()
}

func TestRoom_EnqueueGameResultAsync_WithRedisPath(t *testing.T) {
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

	repo := &trackingRoomRepo{mockRoomRepository: *newMockRoomRepository()}
	h := NewHub(repo, redisStore, config.DefaultTimeoutConfig(), 0, 0, nil)
	r := NewRoom("RES1", h, repo, config.DefaultTimeoutConfig(), 0)
	r.state.SessionID = "sess-res"
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1", ScoreContribution: 5, TapsCount: 1}

	r.enqueueGameResultAsync()
	r.asyncWg.Wait()
}

func TestRoom_CreateGameSessionAsync_StoreError(t *testing.T) {
	repo := &trackingRoomRepo{
		mockRoomRepository: *newMockRoomRepository(),
		createSessionErr:   errors.New("create failed"),
	}
	r := NewRoom("SESS", nil, repo, config.DefaultTimeoutConfig(), 0)
	r.createGameSessionAsync(&domain.GameSession{ID: "s1", LobbyCode: "SESS"})
	r.asyncWg.Wait()
}

func TestRoom_RunGameResultJob_EnqueueError(t *testing.T) {
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
	_ = redisStore.Close()

	h := NewHub(nil, redisStore, config.DefaultTimeoutConfig(), 0, 0, nil)
	r := NewRoom("RQ", h, nil, config.DefaultTimeoutConfig(), 0)
	r.runGameResultJob(gameResultJob{
		sessionID: "s1",
		roomCode:  "RQ",
		payload:   []byte(`{"game_id":"s1"}`),
	})
}
