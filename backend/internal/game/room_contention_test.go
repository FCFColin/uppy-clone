package game

import (
	"sync"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

func TestRoom_ConcurrentMessagesUnderPlaying(t *testing.T) {
	timeouts := config.DefaultTimeoutConfig()
	r := NewRoom("CONT", nil, nil, timeouts, 50)

	r.mu.Lock()
	r.state.Phase = domain.PhasePlaying
	r.state.Players["p1"] = &domain.PlayerState{ID: "p1", Nickname: "p1"}
	r.state.Players["p2"] = &domain.PlayerState{ID: "p2", Nickname: "p2"}
	r.connections["p1"] = &PlayerConn{PlayerID: "p1", Send: make(chan []byte, 256)}
	r.connections["p2"] = &PlayerConn{PlayerID: "p2", Send: make(chan []byte, 256)}
	r.mu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			playerID := "p1"
			if id%2 == 1 {
				playerID = "p2"
			}
			start := time.Now()
			if err := r.HandleMessage(playerID, protocol.MsgPing, nil); err != nil {
				t.Errorf("HandleMessage error: %v", err)
			}
			if d := time.Since(start); d > 25*time.Millisecond {
				t.Errorf("HandleMessage blocked %v", d)
			}
		}(i)
	}
	wg.Wait()

	// Avoid Close() here: test PlayerConns have nil websocket handles.
	r.mu.Lock()
	r.stopTick()
	r.mu.Unlock()
}
