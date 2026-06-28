package game

import (
	"context"

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
				h.logger.Error("deserialize lobby state on restore", "code", ls.Code, "error", err)
				continue
			}
			h.rooms[ls.Code] = room
			toFinalize = append(toFinalize, ls.Code)
			restored++
			h.logger.Info("restored room", "code", ls.Code, "phase", room.state.Phase)
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

func (h *Hub) registerRoomLocked(code string, room *Room) *Room {
	if existing, ok := h.rooms[code]; ok {
		return existing
	}
	h.rooms[code] = room
	return room
}

func (h *Hub) loadOrMaterializeRoom(code string) *Room {
	if h.store == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.timeouts.PGQueryTimeout)
	defer cancel()

	if !h.shouldLocalMaterializeRoom(ctx, code) {
		return nil
	}

	ls, err := h.store.LoadLobbyState(ctx, code)
	if err != nil || ls == nil {
		return nil
	}

	room, err := h.deserializeAndMaterialize(code, []byte(ls.State))
	if err != nil {
		h.logger.Error("deserialize lobby state", "code", code, "error", err)
		return nil
	}

	h.mu.Lock()
	room = h.registerRoomLocked(code, room)
	h.mu.Unlock()
	h.finalizeMaterializedRoom(code)
	return room
}
