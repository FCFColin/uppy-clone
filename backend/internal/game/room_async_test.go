package game

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
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

func TestRoom_CreateGameSessionAsync_NilSession(t *testing.T) {
	repo := &trackingRoomRepo{mockRoomRepository: *newMockRoomRepository()}
	r := NewRoom("TEST1", nil, repo, config.DefaultTimeoutConfig(), 0)
	r.createGameSessionAsync(nil)
	if repo.createSessionCount != 0 {
		t.Fatal("nil session should not call store")
	}
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

func TestRoom_FlushPersistSync(t *testing.T) {
	repo := newMockRoomRepository()
	r := NewRoom("TEST1", nil, repo, config.DefaultTimeoutConfig(), 0)
	r.state.LobbyCode = domain.RoomCode("TEST1")

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
	r.state.LobbyCode = domain.RoomCode("FINAL")

	r.startPersistLoop()
	done := make(chan struct{})
	r.persist.ch <- persistJob{
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

func TestRoom_WritePersistJob_StoreError(_ *testing.T) {
	repo := newMockRoomRepository()
	repo.saveErr = errors.New("save failed")
	r := NewRoom("ERR", nil, repo, config.DefaultTimeoutConfig(), 0)
	r.writePersistJob(persistJob{code: "ERR", stateJSON: []byte("{}")})
}

func TestRoom_EnqueueGameResultAsync_OutboxPath(t *testing.T) {
	repo := &trackingRoomRepo{mockRoomRepository: *newMockRoomRepository()}
	r := NewRoom("RES1", nil, repo, config.DefaultTimeoutConfig(), 0)
	r.state.SessionID = "sess-res"
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1", ScoreContribution: 5, TapsCount: 1}

	r.enqueueGameResultAsync()
	r.asyncWg.Wait()

	if repo.insertOutboxCount != 1 {
		t.Fatalf("insertOutboxCount = %d, want 1", repo.insertOutboxCount)
	}
}


