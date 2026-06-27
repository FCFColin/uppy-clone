package game

import (
	"context"
	"testing"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
)

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
