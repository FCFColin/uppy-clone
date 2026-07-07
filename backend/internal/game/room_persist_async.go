package game

import (
	"github.com/uppy-clone/backend/internal/metrics"
)

// writePersistJob is a bridge for tests that call it directly.
func (r *Room) writePersistJob(job persistJob) {
	if r.persist == nil {
		r.persist = newPersistManager(r, r.logger, &r.asyncWg)
	}
	r.persist.write(job)
}

// startPersistLoop launches the debounced persist worker (once per room).
func (r *Room) startPersistLoop() {
	if r.persist == nil {
		return
	}
	r.persist.startLoop()
	r.persistCh = r.persist.ch
}

func (r *Room) canPersist() bool {
	return r.store != nil && r.persist != nil
}

func (r *Room) serializeState() ([]byte, error) {
	return serializeStateFn(r.state)
}

// asyncSaveState serializes state and queues a debounced persist outside the tick lock.
func (r *Room) asyncSaveState() {
	if !r.canPersist() {
		return
	}
	r.mu.Lock()
	data, err := r.serializeState()
	code := string(r.state.LobbyCode)
	r.mu.Unlock()
	if err != nil {
		r.logger.Error("serialize state for async persist", "error", err)
		return
	}
	r.persist.enqueuePersist(data, code)
}

// requestPersist queues a debounced persist. Caller must hold r.mu.
func (r *Room) requestPersist() {
	if !r.canPersist() {
		return
	}
	data, err := r.serializeState()
	if err != nil {
		r.logger.Error("serialize state for persist", "error", err)
		return
	}
	r.persist.enqueuePersist(data, string(r.state.LobbyCode))
	metrics.SetRoomPersistLag(string(r.state.LobbyCode), 0)
}

// flushPersistSync blocks until a final persist completes (used on Close).
func (r *Room) flushPersistSync() {
	if !r.canPersist() {
		return
	}
	r.mu.Lock()
	data, err := r.serializeState()
	code := string(r.state.LobbyCode)
	r.mu.Unlock()
	if err != nil {
		r.logger.Error("serialize state for final persist", "error", err)
		return
	}
	r.persist.flushSync(data, code)
}

func (r *Room) stopPersist() {
	if r.persist != nil {
		r.persist.stop()
	}
}
