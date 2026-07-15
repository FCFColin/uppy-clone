package middleware

import (
	"net/http"

	"golang.org/x/sync/semaphore"
)

// Bulkhead limits concurrent requests using a weighted semaphore.
type Bulkhead struct {
	sem *semaphore.Weighted
}

// NewBulkhead creates a Bulkhead that allows at most limit concurrent requests.
func NewBulkhead(limit int64) *Bulkhead {
	return &Bulkhead{sem: semaphore.NewWeighted(limit)}
}

// Middleware returns an http.Handler that rejects requests when the bulkhead is full.
func (b *Bulkhead) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !b.sem.TryAcquire(1) {
			http.Error(w, `{"error":"service busy","code":"BULKHEAD_FULL"}`, http.StatusServiceUnavailable)
			return
		}
		defer b.sem.Release(1)
		next.ServeHTTP(w, r)
	})
}

var (
	// AuthBulkhead limits concurrent auth endpoint requests.
	AuthBulkhead = NewBulkhead(10)
	// LobbyBulkhead limits concurrent lobby endpoint requests.
	LobbyBulkhead = NewBulkhead(10)
	// AdminBulkhead limits concurrent admin endpoint requests.
	AdminBulkhead = NewBulkhead(3)
	// WebSocketBulkhead limits concurrent WebSocket connections.
	WebSocketBulkhead = NewBulkhead(50)
)
