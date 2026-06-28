package game

import (
	"context"

	"github.com/uppy-clone/backend/internal/audit"
)

type removeRoomOptions struct {
	audit    bool
	pgDelete bool
	cache    bool
	logMsg   string
}

func (h *Hub) removeRoomFromMemory(code string, logMsg string) *Room {
	var room *Room
	if r, ok := h.rooms[code]; ok {
		room = r
		delete(h.rooms, code)
		h.unsubscribeRoomLocked(code)
		if logMsg != "" {
			h.logger.Info(logMsg, "code", code)
		}
	}
	return room
}

func (h *Hub) finalizeRoomRemoval(ctx context.Context, code string, room *Room, opts removeRoomOptions) {
	if room != nil {
		room.Close()
	}

	if opts.pgDelete && h.store != nil {
		pctx, cancel := context.WithTimeout(context.Background(), h.timeouts.PGQueryTimeout)
		defer cancel()
		_ = h.store.DeleteLobbyState(pctx, code)
	}

	h.unregisterRoomFromRedis(code)

	if opts.cache {
		h.invalidateLobbyReadCaches(code)
	}

	if opts.audit {
		audit.Log(ctx, audit.AuditEntry{
			Action:   "room.delete",
			ActorID:  "system",
			Resource: "room/" + code,
			Before:   map[string]interface{}{"code": code},
		})
	}
}

func (h *Hub) removeRooms(codes []string, opts removeRoomOptions) {
	if len(codes) == 0 {
		return
	}

	type codeRoom struct {
		code string
		room *Room
	}
	var batch []codeRoom

	h.mu.Lock()
	for _, code := range codes {
		if room := h.removeRoomFromMemory(code, opts.logMsg); room != nil {
			batch = append(batch, codeRoom{code: code, room: room})
		}
	}
	h.mu.Unlock()

	for _, item := range batch {
		h.finalizeRoomRemoval(context.Background(), item.code, item.room, opts)
	}
}

func (h *Hub) removeSingleRoom(ctx context.Context, code string, opts removeRoomOptions) {
	var room *Room
	h.mu.Lock()
	room = h.removeRoomFromMemory(code, opts.logMsg)
	h.mu.Unlock()
	h.finalizeRoomRemoval(ctx, code, room, opts)
}
