package game

import (
	"context"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
)

// CleanupLoop 定期清理空房间
func (h *Hub) CleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(cleanupLoopInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.cleanupOnce()
		}
	}
}

func (h *Hub) cleanupOnce() {
	codes := h.snapshotRoomCodes()
	now := time.Now().UnixMilli()

	var toCleanup []string
	for _, code := range codes {
		h.mu.RLock()
		room, ok := h.rooms[code]
		h.mu.RUnlock()
		if !ok {
			continue
		}
		if shouldCleanupRoom(room, now) {
			toCleanup = append(toCleanup, code)
		}
	}

	h.removeRooms(toCleanup, removeRoomOptions{
		pgDelete: true,
		cache:    true,
		logMsg:   "cleaned up empty room",
	})
}

// cleanupIntervalForTest overrides CleanupInterval in unit tests.
var cleanupIntervalForTest time.Duration

// snapshotRoomCodesHook, when set, replaces snapshotRoomCodes behavior in unit tests.
var snapshotRoomCodesHook func(*Hub) []string

func (h *Hub) snapshotRoomCodes() []string {
	if snapshotRoomCodesHook != nil {
		return snapshotRoomCodesHook(h)
	}
	h.mu.RLock()
	codes := make([]string, 0, len(h.rooms))
	for code := range h.rooms {
		codes = append(codes, code)
	}
	h.mu.RUnlock()
	return codes
}

func shouldCleanupRoom(room *Room, now int64) bool {
	room.mu.RLock()
	defer room.mu.RUnlock()

	phase := room.state.Phase
	playerCount := len(room.state.Players)
	hasConnections := len(room.connections) > 0

	if phase == domain.PhaseWaiting && !hasConnections {
		return true
	}
	if playerCount == 0 && !hasConnections {
		return true
	}
	if !hasConnections && playerCount > 0 {
		return allPlayersDisconnectedExpired(room.state.Players, now)
	}
	return false
}

func allPlayersDisconnectedExpired(players map[string]*domain.PlayerState, now int64) bool {
	for _, p := range players {
		if p.Disconnected && p.DisconnectedAt != nil {
			if !reconnectGraceExpired(*p.DisconnectedAt, now) {
				return false
			}
		} else {
			return false
		}
	}
	return true
}

func cleanupLoopInterval() time.Duration {
	if cleanupIntervalForTest > 0 {
		return cleanupIntervalForTest
	}
	return config.CleanupInterval
}
