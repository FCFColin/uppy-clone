package game

import (
	"context"

	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/metrics"
)

// RemoveRoom removes a room.
func (h *Hub) RemoveRoom(ctx context.Context, code string) {
	h.removeSingleRoom(ctx, code, removeRoomOptions{
		audit:    true,
		pgDelete: true,
		cache:    true,
		logMsg:   "room removed",
	})
}

// CloseAllRooms closes all active rooms, ensuring state is persisted.
func (h *Hub) CloseAllRooms() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for code, room := range h.rooms {
		room.Close()
		delete(h.rooms, code)
		h.logger.Info("room closed on shutdown", codeKey, code)
	}
	metrics.ActiveRooms.Set(0)
}

// RoomCount 返回当前房间数量
func (h *Hub) RoomCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.rooms)
}

// PlayerCount 返回所有房间中的总玩家数
func (h *Hub) PlayerCount() int {
	h.mu.RLock()
	rooms := make([]*Room, 0, len(h.rooms))
	for _, room := range h.rooms {
		rooms = append(rooms, room)
	}
	h.mu.RUnlock()
	total := 0
	for _, room := range rooms {
		room.mu.RLock()
		total += len(room.state.Players)
		room.mu.RUnlock()
	}
	return total
}

// PhaseCounts returns the number of rooms in each game phase.
func (h *Hub) PhaseCounts() map[string]int {
	h.mu.RLock()
	rooms := make([]*Room, 0, len(h.rooms))
	for _, room := range h.rooms {
		rooms = append(rooms, room)
	}
	h.mu.RUnlock()
	counts := make(map[string]int)
	for _, room := range rooms {
		room.mu.RLock()
		phase := string(room.state.Phase)
		room.mu.RUnlock()
		counts[phase]++
	}
	return counts
}

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
	rooms := make([]*Room, 0, len(h.rooms))
	codes := make([]string, 0, len(h.rooms))
	for code, room := range h.rooms {
		rooms = append(rooms, room)
		codes = append(codes, code)
	}
	h.mu.RUnlock()
	results := make([]string, 0, len(rooms))
	for i, room := range rooms {
		if roomJoinable(room, h.maxPlayersPerRoom) {
			results = append(results, codes[i])
		}
	}
	return results
}

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
		metrics.ActiveRooms.Set(float64(len(h.rooms)))
		if logMsg != "" {
			h.logger.Info(logMsg, codeKey, code)
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
			Action:    "room.delete",
			ActorType: audit.ActorTypeSystem,
			ActorID:   "system",
			Resource:  "room/" + code,
			Before:    map[string]interface{}{codeKey: code},
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

func (h *Hub) registerRoomLocked(code string, room *Room) *Room {
	if existing, ok := h.rooms[code]; ok {
		return existing
	}
	h.rooms[code] = room
	return room
}

// loadOrMaterializeRoom 从 PG 加载房间并注册到内存（I/O 在锁外执行）。
func (h *Hub) loadOrMaterializeRoom(code string) *Room {
	if h.store == nil {
		return nil
	}

	// I/O operations outside lock
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
		h.logger.Error("deserialize lobby state", codeKey, code, "error", err)
		return nil
	}

	// Register under lock
	h.mu.Lock()
	room = h.registerRoomLocked(code, room)
	h.mu.Unlock()

	if room != nil {
		h.finalizeMaterializedRoom(code)
	}
	return room
}
