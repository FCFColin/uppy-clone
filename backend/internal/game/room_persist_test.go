package game

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
)

func TestSaveStateWithError_PersistsLobbyMetadata(t *testing.T) {
	t.Parallel()
	repo := newMockRoomRepository()
	room := &Room{
		state:  NewGameState("PERSIST", testRNG()),
		store:  repo,
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
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
		state:  NewGameState("TEST", testRNG()),
		store:  nil,
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
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
		state:  NewGameState("TEST", testRNG()),
		store:  repo,
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
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
		state:  NewGameState("TEST", testRNG()),
		store:  repo,
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	err := room.saveStateWithError()
	if err == nil {
		t.Fatal("saveStateWithError should return error when store fails")
	}
}

func TestSaveState_NilStoreDoesNotPanic(t *testing.T) {
	t.Parallel()
	room := &Room{
		state:  NewGameState("TEST", testRNG()),
		store:  nil,
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	room.saveState()
}

func TestSaveState_StoreErrorDoesNotPanic(t *testing.T) {
	t.Parallel()
	repo := newMockRoomRepository()
	repo.saveErr = errors.New("db unavailable")
	room := &Room{
		state:  NewGameState("TEST", testRNG()),
		store:  repo,
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	room.saveState()
}
