package game

import (
	"context"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
)

// RestoreRooms 从 PostgreSQL 加载本实例应拥有的活跃房间（启动时调用）。
func (h *Hub) RestoreRooms() error {
	if h.store == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.timeouts.PGRequestTimeout)
	defer cancel()

	cursor := ""
	const pageSize = 100
	restored := 0

	for {
		result, err := h.store.LoadAllActiveLobbies(ctx, pageSize, cursor)
		if err != nil {
			return err
		}
		if len(result.Lobbies) == 0 {
			break
		}

		var toFinalize []string
		h.mu.Lock()
		for _, ls := range result.Lobbies {
			if _, ok := h.rooms[ls.Code]; ok {
				continue
			}
			if !h.shouldLocalMaterializeRoom(ctx, ls.Code) {
				continue
			}
			room, err := h.deserializeAndMaterialize(ls.Code, []byte(ls.State))
			if err != nil {
				h.logger.Error("deserialize lobby state on restore", codeKey, ls.Code, "error", err)
				continue
			}
			h.rooms[ls.Code] = room
			toFinalize = append(toFinalize, ls.Code)
			restored++
			h.logger.Info("restored room", codeKey, ls.Code, "phase", room.state.Phase)
		}
		h.mu.Unlock()

		for _, code := range toFinalize {
			h.finalizeMaterializedRoom(code)
		}

		if len(result.Lobbies) < pageSize {
			break
		}
		cursor = result.Lobbies[len(result.Lobbies)-1].Code
	}

	h.logger.Info("rooms restored", "count", restored)
	return nil
}

func (h *Hub) materializeRoom(code string, state *domain.GameState) *Room {
	room := NewRoom(code, h, h.store, h.timeouts, h.maxPlayersPerRoom)
	room.state = state
	room.rng = newSeededRNG(state.RNGSeed)
	for _, p := range state.Players {
		room.usedNames[p.Nickname] = true
	}
	return room
}

func (h *Hub) deserializeAndMaterialize(code string, stateJSON []byte) (*Room, error) {
	state, err := DeserializeState(stateJSON)
	if err != nil {
		return nil, err
	}
	return h.materializeRoom(code, state), nil
}

// CleanupLoop 定期清理空房间
func (h *Hub) CleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(h.cleanupLoopInterval())
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
	type roomEntry struct {
		code string
		room *Room
	}

	h.mu.RLock()
	entries := make([]roomEntry, 0, len(h.rooms))
	for code, room := range h.rooms {
		entries = append(entries, roomEntry{code: code, room: room})
	}
	h.mu.RUnlock()

	now := time.Now().UnixMilli()
	var toCleanup []string
	for _, entry := range entries {
		if shouldCleanupRoom(entry.room, now) {
			toCleanup = append(toCleanup, entry.code)
		}
	}

	h.removeRooms(toCleanup, removeRoomOptions{
		pgDelete: true,
		cache:    true,
		logMsg:   "cleaned up empty room",
	})
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
		if !p.Disconnected || p.DisconnectedAt == nil {
			return false
		}
		if now-*p.DisconnectedAt <= domain.ReconnectGraceMs {
			return false
		}
	}
	return true
}

func (h *Hub) cleanupLoopInterval() time.Duration {
	if h.cleanupInterval > 0 {
		return h.cleanupInterval
	}
	return config.CleanupInterval
}
