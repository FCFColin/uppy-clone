package game

import (
	"context"

	"github.com/uppy-clone/backend/internal/domain"
)

// MatchRoom finds a joinable waiting room or creates a new one for quick play.
func (h *Hub) MatchRoom(ctx context.Context) (string, error) {
	candidates := h.joinableRoomCodes()
	for _, code := range candidates {
		h.mu.RLock()
		room, ok := h.rooms[code]
		h.mu.RUnlock()
		if !ok {
			continue
		}
		room.mu.RLock()
		joinable := room.state.Phase == domain.PhaseWaiting &&
			len(room.state.Players) < h.maxPlayersPerRoom &&
			len(room.connections) < h.maxPlayersPerRoom
		room.mu.RUnlock()
		if joinable {
			return code, nil
		}
	}

	return h.CreateRoom(ctx)
}

func (h *Hub) joinableRoomCodes() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	codes := make([]string, 0, len(h.rooms))
	for code, room := range h.rooms {
		room.mu.RLock()
		if room.state.Phase == domain.PhaseWaiting &&
			len(room.state.Players) < h.maxPlayersPerRoom {
			codes = append(codes, code)
		}
		room.mu.RUnlock()
	}
	return codes
}
