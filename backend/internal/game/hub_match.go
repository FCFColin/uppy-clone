package game

import (
	"context"

	"github.com/uppy-clone/backend/internal/domain"
)

// MatchRoom finds a joinable waiting room or creates a new one for quick play.
func (h *Hub) MatchRoom(ctx context.Context) (string, error) {
	h.mu.RLock()
	for code, room := range h.rooms {
		room.mu.RLock()
		joinable := room.state.Phase == domain.PhaseWaiting &&
			len(room.state.Players) < h.maxPlayersPerRoom &&
			len(room.connections) < h.maxPlayersPerRoom
		room.mu.RUnlock()
		if joinable {
			h.mu.RUnlock()
			return code, nil
		}
	}
	h.mu.RUnlock()

	return h.CreateRoom(ctx)
}
