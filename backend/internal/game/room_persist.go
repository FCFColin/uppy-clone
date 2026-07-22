package game

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/metrics"
)

// ─── PersistManager (debounced async persistence) ───────────────────

const persistQueueSize = 8
const persistDebounce = 100 * time.Millisecond

type persistJob struct {
	code      string
	stateJSON []byte
	final     bool
	done      chan struct{}
}

// PersistManager handles debounced async PostgreSQL persistence for game room state.
type PersistManager struct {
	source  *Room
	logger  *slog.Logger
	asyncWG *sync.WaitGroup

	ch            chan persistJob
	once          sync.Once
	mu            sync.RWMutex
	lastPersistAt time.Time
}

func newPersistManager(source *Room, logger *slog.Logger, asyncWG *sync.WaitGroup) *PersistManager {
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
				// game-017: Drain stale timer value before Reset to prevent
				// spurious wakeups on Go <1.23.
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(persistDebounce)
				timerC = timer.C
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
		// game-018: Log and metric when persist queue is full — previously
		// silently dropped, making state persistence gaps invisible.
		m.logger.Warn("persist queue full, dropping state save", codeKey, code)
		metrics.RoomPersistDropped.Inc()
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
	select {
	case m.ch <- job:
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			m.logger.Warn("flushSync timed out waiting for persist", codeKey, code)
		}
	case <-time.After(5 * time.Second):
		m.logger.Warn("flushSync timed out enqueueing persist", codeKey, code)
	}
}

func (m *PersistManager) stop() {
	m.once.Do(func() {})
	if m.ch != nil {
		close(m.ch)
	}
}

// ─── Room Persist Methods ────────────────────────────────────────────

func newLobbyState(code string, stateJSON []byte) *domain.LobbyState {
	now := time.Now().UnixMilli()
	return &domain.LobbyState{
		ID:        domain.UUID(),
		Code:      code,
		State:     string(stateJSON),
		UpdatedAt: now,
		CreatedAt: now,
	}
}

// saveStateWithError persists state to PostgreSQL and returns any error.
// game-027: Caller must hold r.mu (write lock) — this function reads r.state
// without acquiring the lock to avoid recursive locking. All current callers
// (flushSync, requestPersist→write) hold the lock or operate on a serialized copy.
func (r *Room) saveStateWithError() error {
	if r.store == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.timeouts.PGQueryTimeout)
	defer cancel()

	data, err := SerializeState(r.state)
	if err != nil {
		return fmt.Errorf("serialize state: %w", err)
	}

	ls := newLobbyState(string(r.state.LobbyCode), data)
	if err := r.store.SaveLobbyState(ctx, ls); err != nil {
		return fmt.Errorf("save lobby state: %w", err)
	}
	return nil
}

// writePersistJob is a bridge for tests that call it directly.
func (r *Room) writePersistJob(job persistJob) {
	if r.persist == nil {
		r.persist = newPersistManager(r, r.logger, &r.asyncWg)
	}
	r.persist.write(job)
}

func (r *Room) startPersistLoop() {
	if r.persist == nil {
		return
	}
	r.persist.startLoop()
}

func (r *Room) canPersist() bool {
	return r.store != nil && r.persist != nil
}

func (r *Room) serializeState() ([]byte, error) {
	return SerializeState(r.state)
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

// createGameSessionAsync inserts a game session row without blocking the room lock.
func (r *Room) createGameSessionAsync(session *domain.GameSession) {
	if r.store == nil || session == nil {
		return
	}
	r.asyncWg.Add(1)
	go func() {
		defer r.asyncWg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), r.timeouts.PGQueryTimeout)
		defer cancel()
		if err := r.store.CreateGameSession(ctx, session); err != nil {
			r.logger.Warn("create game session failed (game result worker will handle persistence)",
				"error", err,
				"room_code", session.LobbyCode)
		}
	}()
}
