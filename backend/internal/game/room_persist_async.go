package game

import (
	"context"
	"time"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/idgen"
	"github.com/uppy-clone/backend/internal/metrics"
)

const persistQueueSize = 8
const persistDebounce = 100 * time.Millisecond

type persistJob struct {
	code      string
	stateJSON []byte
	final     bool
	done      chan struct{}
}

// startPersistLoop launches the debounced persist worker (once per room).
func (r *Room) startPersistLoop() {
	r.persistOnce.Do(func() {
		r.persistCh = make(chan persistJob, persistQueueSize)
		r.asyncWg.Add(1)
		go r.runPersistLoop()
	})
}

func (r *Room) runPersistLoop() {
	defer r.asyncWg.Done()
	var pending *persistJob
	var timer *time.Timer
	var timerC <-chan time.Time

	flush := func() {
		if pending == nil {
			return
		}
		job := pending
		pending = nil
		if timer != nil {
			timer.Stop()
			timerC = nil
		}
		r.writePersistJob(*job)
	}

	for {
		select {
		case job, ok := <-r.persistCh:
			if !ok {
				flush()
				return
			}
			if job.final {
				flush()
				r.writePersistJob(job)
				continue
			}
			pending = &job
			if timer == nil {
				timer = time.NewTimer(persistDebounce)
				timerC = timer.C
			} else {
				timer.Reset(persistDebounce)
			}
		case <-timerC:
			flush()
		}
	}
}

func (r *Room) writePersistJob(job persistJob) {
	if r.store == nil {
		if job.done != nil {
			close(job.done)
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.timeouts.PGQueryTimeout)
	defer cancel()
	ls := &domain.LobbyState{
		ID:        idgen.UUID(),
		Code:      job.code,
		State:     string(job.stateJSON),
		UpdatedAt: time.Now().UnixMilli(),
		CreatedAt: time.Now().UnixMilli(),
	}
	err := r.store.SaveLobbyState(ctx, ls)
	if err != nil {
		r.logger.Error("async save state", "error", err)
	} else {
		r.persistMu.Lock()
		r.lastPersistAt = time.Now()
		r.persistMu.Unlock()
		metrics.SetRoomPersistLag(job.code, 0)
	}
	if job.done != nil {
		close(job.done)
	}
}

// requestPersist queues a debounced persist. Caller must hold r.mu.
func (r *Room) requestPersist() {
	if r.store == nil {
		return
	}
	data, err := SerializeState(r.state)
	if err != nil {
		r.logger.Error("serialize state for persist", "error", err)
		return
	}
	r.startPersistLoop()
	job := persistJob{
		code:      r.state.LobbyCode,
		stateJSON: append([]byte(nil), data...),
	}
	select {
	case r.persistCh <- job:
	default:
		// Coalesce: drop intermediate job; latest state will follow on next tick.
	}
	r.persistMu.RLock()
	if !r.lastPersistAt.IsZero() {
		metrics.SetRoomPersistLag(r.state.LobbyCode, time.Since(r.lastPersistAt))
	}
	r.persistMu.RUnlock()
}

// flushPersistSync blocks until a final persist completes (used on Close).
func (r *Room) flushPersistSync() {
	if r.store == nil {
		return
	}
	r.mu.Lock()
	data, err := SerializeState(r.state)
	code := r.state.LobbyCode
	r.mu.Unlock()
	if err != nil {
		r.logger.Error("serialize state for final persist", "error", err)
		return
	}

	r.startPersistLoop()
	done := make(chan struct{})
	job := persistJob{
		code:      code,
		stateJSON: data,
		final:     true,
		done:      done,
	}
	r.persistCh <- job
	<-done
}

func (r *Room) stopPersist() {
	r.persistOnce.Do(func() {})
	if r.persistCh != nil {
		close(r.persistCh)
	}
}
