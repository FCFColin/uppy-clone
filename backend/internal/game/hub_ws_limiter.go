package game

import (
	"sync/atomic"

	"github.com/uppy-clone/backend/internal/metrics"
)

// ErrWSConnectionLimit 全局 WebSocket 连接数已达上限
var ErrWSConnectionLimit = &wsConnectionLimitError{}

type wsConnectionLimitError struct{}

func (e *wsConnectionLimitError) Error() string { return "websocket connection limit reached" }

// CanAcceptWSConnection 检查是否可以接受新的 WebSocket 连接
func (h *Hub) CanAcceptWSConnection() bool {
	return atomic.LoadInt64(&h.wsConnCount) < int64(h.maxWSConnections)
}

// TryReserveWSConnection atomically reserves a WS slot before upgrade (avoids TOCTOU).
// Call DecrementWSConnection if upgrade/join fails after a successful reserve.
func (h *Hub) TryReserveWSConnection() bool {
	for {
		current := atomic.LoadInt64(&h.wsConnCount)
		if current >= int64(h.maxWSConnections) {
			return false
		}
		if atomic.CompareAndSwapInt64(&h.wsConnCount, current, current+1) {
			metrics.WSConnections.Set(float64(current + 1))
			return true
		}
	}
}

// IncrementWSConnection increments the global WebSocket connection counter.
func (h *Hub) IncrementWSConnection() {
	count := atomic.AddInt64(&h.wsConnCount, 1)
	metrics.WSConnections.Set(float64(count))
}

// DecrementWSConnection decrements the global WebSocket connection counter.
func (h *Hub) DecrementWSConnection() {
	count := atomic.AddInt64(&h.wsConnCount, -1)
	metrics.WSConnections.Set(float64(count))
}

// WSConnCount returns the current number of active WebSocket connections.
func (h *Hub) WSConnCount() int64 {
	return atomic.LoadInt64(&h.wsConnCount)
}

// MaxWSConnections returns the configured global WebSocket connection limit.
func (h *Hub) MaxWSConnections() int {
	return h.maxWSConnections
}

// MaxPlayersPerRoom returns the configured per-room player limit.
func (h *Hub) MaxPlayersPerRoom() int {
	return h.maxPlayersPerRoom
}
