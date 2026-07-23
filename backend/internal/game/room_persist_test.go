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

func TestSaveStateWithError_PersistsLobbyMetadata(t *testing.T) {
	t.Parallel()
	repo := newMockRoomRepository()
	room := &Room{
		state:    NewGameState("PERSIST", 42, testRNG()),
		store:    repo,
		logger:   slog.New(slog.NewTextHandler(os.Stderr, nil)),
		timeouts: config.DefaultTimeoutConfig(),
	}
	if err := room.saveStateWithError(); err != nil {
		t.Fatalf("saveStateWithError: %v", err)
	}
	stored, err := repo.LoadLobbyState(context.Background(), "PERSIST")
	if err != nil {
		t.Fatalf("LoadLobbyState: %v", err)
	}
	if stored.ID == "" || stored.Code != "PERSIST" {
		t.Fatalf("stored = %+v", stored)
	}
}

func TestSaveStateWithError_StoreError(t *testing.T) {
	t.Parallel()
	repo := newMockRoomRepository()
	repo.saveErr = errors.New("db unavailable")
	room := &Room{
		state:    NewGameState("TEST", 42, testRNG()),
		store:    repo,
		logger:   slog.New(slog.NewTextHandler(os.Stderr, nil)),
		timeouts: config.DefaultTimeoutConfig(),
	}

	err := room.saveStateWithError()
	if err == nil {
		t.Fatal("saveStateWithError should return error when store fails")
	}
}

type trackingRoomRepo struct {
	mockRoomRepository
	createSessionErr   error
	createSessionCount int
}

func (t *trackingRoomRepo) CreateGameSession(_ context.Context, _ *domain.GameSession) error {
	t.createSessionCount++
	return t.createSessionErr
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
