package game

import (
	"context"

	"github.com/uppy-clone/backend/internal/domain"
)

// roomJoinable reports whether a waiting room can accept another player.
func roomJoinable(room *Room, maxPlayers int) bool {
	room.mu.RLock()
	defer room.mu.RUnlock()
	return room.state.Phase == domain.PhaseWaiting &&
		len(room.state.Players) < maxPlayers &&
		len(room.connections) < maxPlayers
}

// MatchRoom finds a joinable waiting room or creates a new one for quick play.
func (h *Hub) MatchRoom(ctx context.Context) (string, error) {
	for _, code := range h.joinableRoomCodes() {
		h.mu.RLock()
		room := h.rooms[code]
		h.mu.RUnlock()
		if room != nil && roomJoinable(room, h.maxPlayersPerRoom) {
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
		if roomJoinable(room, h.maxPlayersPerRoom) {
			codes = append(codes, code)
		}
	}
	return codes
}
