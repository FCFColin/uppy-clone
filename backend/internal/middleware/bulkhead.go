package middleware

import (
	"net/http"

	"golang.org/x/sync/semaphore"
)

type Bulkhead struct {
	sem *semaphore.Weighted
}

func NewBulkhead(limit int64) *Bulkhead {
	return &Bulkhead{sem: semaphore.NewWeighted(limit)}
}

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
	AuthBulkhead       = NewBulkhead(10)
	LobbyBulkhead      = NewBulkhead(10)
	AdminBulkhead      = NewBulkhead(3)
	WebSocketBulkhead  = NewBulkhead(50)
)
