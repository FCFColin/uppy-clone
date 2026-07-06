package game

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/uppy-clone/backend/internal/config"
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

// PersistSource provides game state serialization for persistence.
type PersistSource interface {
	SerializeStateJSON() ([]byte, string, error)
	Store() RoomRepository
	Timeouts() config.TimeoutConfig
	LobbyCode() string
}

// PersistManager handles debounced async PostgreSQL persistence for game room state.
type PersistManager struct {
	source    PersistSource
	logger    *slog.Logger
	asyncWG   *sync.WaitGroup

	ch           chan persistJob
	once         sync.Once
	mu           sync.RWMutex
	lastPersistAt time.Time
}

func newPersistManager(source PersistSource, logger *slog.Logger, asyncWG *sync.WaitGroup) *PersistManager {
	return &PersistManager{
		source:  source,
		logger:  logger,
		asyncWG: asyncWG,
	}
}

func (m *PersistManager) startLoop() {
	m.once.Do(func() {
		m.ch = make(chan persistJob, persistQueueSize)
		m.asyncWG.Add(1)
		go m.runLoop()
	})
}

func (m *PersistManager) runLoop() {
	defer m.asyncWG.Done()
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
		m.write(*job)
	}

	for {
		select {
		case job, ok := <-m.ch:
			if !ok {
				flush()
				return
			}
			if job.final {
				flush()
				m.write(job)
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

func (m *PersistManager) write(job persistJob) {
	store := m.source.Store()
	if store == nil {
		if job.done != nil {
			close(job.done)
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), m.source.Timeouts().PGQueryTimeout)
	defer cancel()
	ls := newLobbyState(job.code, job.stateJSON)
	err := store.SaveLobbyState(ctx, ls)
	if err != nil {
		m.logger.Error("async save state", "error", err)
	} else {
		m.mu.Lock()
		m.lastPersistAt = time.Now()
		m.mu.Unlock()
		metrics.SetRoomPersistLag(job.code, 0)
	}
	if job.done != nil {
		close(job.done)
	}
}

func (m *PersistManager) enqueuePersist(data []byte, code string) {
	m.startLoop()
	job := persistJob{
		code:      code,
		stateJSON: data,
	}
	select {
	case m.ch <- job:
	default:
	}
	m.mu.RLock()
	if !m.lastPersistAt.IsZero() {
		metrics.SetRoomPersistLag(code, time.Since(m.lastPersistAt))
	}
	m.mu.RUnlock()
}

func (m *PersistManager) flushSync(data []byte, code string) {
	m.startLoop()
	done := make(chan struct{})
	job := persistJob{
		code:      code,
		stateJSON: data,
		final:     true,
		done:      done,
	}
	m.ch <- job
	<-done
}

func (m *PersistManager) stop() {
	m.once.Do(func() {})
	if m.ch != nil {
		close(m.ch)
	}
}
