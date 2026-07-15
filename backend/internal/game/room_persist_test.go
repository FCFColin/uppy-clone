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
		state:              NewGameState("PERSIST", 42, testRNG()),
		store:              repo,
		logger:             slog.New(slog.NewTextHandler(os.Stderr, nil)),
		serializeStateFunc: SerializeState,
		timeouts:           config.DefaultTimeoutConfig(),
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

func TestSaveStateWithError_NilStore(t *testing.T) {
	t.Parallel()
	room := &Room{
		state:              NewGameState("TEST", 42, testRNG()),
		store:              nil,
		logger:             slog.New(slog.NewTextHandler(os.Stderr, nil)),
		serializeStateFunc: SerializeState,
	}

	err := room.saveStateWithError()
	if err != nil {
		t.Errorf("saveStateWithError with nil store should return nil, got: %v", err)
	}
}

func TestSaveStateWithError_StoreSuccess(t *testing.T) {
	t.Parallel()
	repo := newMockRoomRepository()
	room := &Room{
		state:              NewGameState("TEST", 42, testRNG()),
		store:              repo,
		logger:             slog.New(slog.NewTextHandler(os.Stderr, nil)),
		serializeStateFunc: SerializeState,
		timeouts:           config.DefaultTimeoutConfig(),
	}

	err := room.saveStateWithError()
	if err != nil {
		t.Errorf("saveStateWithError should succeed, got: %v", err)
	}
	if repo.saveCount != 1 {
		t.Errorf("saveCount = %d, want 1", repo.saveCount)
	}
}

func TestSaveStateWithError_StoreError(t *testing.T) {
	t.Parallel()
	repo := newMockRoomRepository()
	repo.saveErr = errors.New("db unavailable")
	room := &Room{
		state:              NewGameState("TEST", 42, testRNG()),
		store:              repo,
		logger:             slog.New(slog.NewTextHandler(os.Stderr, nil)),
		serializeStateFunc: SerializeState,
		timeouts:           config.DefaultTimeoutConfig(),
	}

	err := room.saveStateWithError()
	if err == nil {
		t.Fatal("saveStateWithError should return error when store fails")
	}
}

func TestSaveState_NilStoreDoesNotPanic(t *testing.T) {
	t.Parallel()
	room := &Room{
		state:  NewGameState("TEST", 42, testRNG()),
		store:  nil,
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	room.requestPersist()
}

func TestSaveState_StoreErrorDoesNotPanic(t *testing.T) {
	t.Parallel()
	repo := newMockRoomRepository()
	repo.saveErr = errors.New("db unavailable")
	room := &Room{
		state:  NewGameState("TEST", 42, testRNG()),
		store:  repo,
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	room.requestPersist()
}

// --- coverage gap 补充用例 ---

func TestRoom_RequestPersist_UpdatesPersistLag(_ *testing.T) {
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

func TestSaveStateWithError_SerializeError(t *testing.T) {
	repo := newMockRoomRepository()
	r := NewRoom("SER", nil, repo, config.DefaultTimeoutConfig(), 0)
	r.serializeStateFunc = func(*domain.GameState) ([]byte, error) {
		return nil, errors.New("serialize failed")
	}
	if err := r.saveStateWithError(); err == nil {
		t.Fatal("expected serialize error")
	}
}

func TestRoom_RequestPersist_SerializeError(_ *testing.T) {
	repo := newMockRoomRepository()
	r := NewRoom("RSE", nil, repo, config.DefaultTimeoutConfig(), 0)
	r.serializeStateFunc = func(*domain.GameState) ([]byte, error) {
		return nil, errors.New("serialize failed")
	}
	r.mu.Lock()
	r.requestPersist()
	r.mu.Unlock()
}

func TestRoom_FlushPersistSync_SerializeError(_ *testing.T) {
	repo := newMockRoomRepository()
	r := NewRoom("FSE", nil, repo, config.DefaultTimeoutConfig(), 0)
	r.serializeStateFunc = func(*domain.GameState) ([]byte, error) {
		return nil, errors.New("serialize failed")
	}
	r.flushPersistSync()
}
