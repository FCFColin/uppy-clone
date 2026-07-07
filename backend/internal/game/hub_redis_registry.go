package game

import (
	"context"
	"encoding/json"
	"time"

	"github.com/uppy-clone/backend/internal/domain"
)

const roomRegistryTTL = 24 * time.Hour

func (h *Hub) shouldLocalMaterializeRoom(ctx context.Context, code string) bool {
	if 	h.cache == nil {
		return true
	}
	info, err := 	h.cache.GetRoomRegistry(ctx, code)
	if err != nil {
		h.logger.Warn("room registry lookup failed", "code", code, "error", err)
		return false
	}
	if info == nil || info.Instance == "" {
		return true
	}
	return info.Instance == h.instanceID
}

func (h *Hub) finalizeMaterializedRoom(code string) {
	h.subscribeRoom(code)
	h.registerRoomInRedis(code)
}

func (h *Hub) cacheOp(fn func(context.Context) error) {
	if h.cache == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), h.timeouts.RedisConnectTimeout)
	defer cancel()
	_ = fn(ctx)
}

func (h *Hub) registerRoomInRedis(code string) {
	h.cacheOp(func(ctx context.Context) error {
		data, _ := json.Marshal(domain.RoomRegistryInfo{
			Code:      code,
			Instance:  h.instanceID,
			Address:   instanceAddress(),
			CreatedAt: time.Now().UnixMilli(),
		})
		return h.cache.RegisterRoom(ctx, code, data, roomRegistryTTL)
	})
}

func (h *Hub) unregisterRoomFromRedis(code string) {
	h.cacheOp(func(ctx context.Context) error {
		return h.cache.UnregisterRoom(ctx, code)
	})
}

func (h *Hub) subscribeRoom(code string) {
	if h.broadcaster == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, exists := h.subscriptions[code]; exists {
		return
	}
	unsub, err := h.broadcaster.Subscribe(code, func(msg BroadcastMessage) {
		h.handleRemoteBroadcast(code, msg)
	})
	if err != nil {
		h.logger.Warn("subscribe room broadcast failed", "code", code, "error", err)
		return
	}
	h.subscriptions[code] = unsub
}

func (h *Hub) unsubscribeRoomLocked(code string) {
	if unsub, ok := h.subscriptions[code]; ok {
		unsub()
		delete(h.subscriptions, code)
	}
}

func (h *Hub) handleRemoteBroadcast(roomCode string, msg BroadcastMessage) {
	if msg.ExcludeInstance == h.instanceID {
		return
	}
	h.mu.RLock()
	room, ok := h.rooms[roomCode]
	h.mu.RUnlock()
	if !ok {
		return
	}
	room.mu.Lock()
	room.broadcastLocal(msg.Payload, msg.ExcludePlayer)
	room.mu.Unlock()
}
