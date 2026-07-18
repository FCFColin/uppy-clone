package game

import (
	"context"
	"time"
)

// GameObserver receives notifications about game domain events for
// cross-cutting concerns (metrics, audit) without coupling the game
// package to those infrastructure packages.
//
// The game package calls these methods at specific domain events.
// The production implementation (in the server package) translates
// them into Prometheus metric updates and audit log entries.
// Tests use NoopGameObserver (the default) to avoid infrastructure deps.
type GameObserver interface {
	// Room lifecycle metrics
	SetActiveRooms(count int)
	IncGameSessions()

	// Performance metrics
	RecordRoomLockHold(reason string, d time.Duration)
	RecordGameTick(d time.Duration)
	RecordWSMessage(msgName string, d time.Duration)

	// Queue / buffer metrics
	SetOutboundQueueDepth(lobbyCode string, depth int)
	IncWSMessageDropped(lobbyCode string)
	IncPersistDropped()

	// Persist metrics
	SetPersistLag(code string, d time.Duration)
	IncGameResultMarshalFailures()

	// Business metrics
	IncNicknameConfirm(accepted bool)

	// Audit
	AuditRoomCreate(ctx context.Context, code string, maxPlayers int)
	AuditRoomDelete(ctx context.Context, code string)
}

// NoopGameObserver implements GameObserver with no-op methods.
// It is the zero-value default for Hub.observer.
type NoopGameObserver struct{}

// SetActiveRooms is a no-op to satisfy GameObserver.
func (NoopGameObserver) SetActiveRooms(int) {}

// IncGameSessions is a no-op to satisfy GameObserver.
func (NoopGameObserver) IncGameSessions() {}

// RecordRoomLockHold is a no-op to satisfy GameObserver.
func (NoopGameObserver) RecordRoomLockHold(string, time.Duration) {}

// RecordGameTick is a no-op to satisfy GameObserver.
func (NoopGameObserver) RecordGameTick(time.Duration) {}

// RecordWSMessage is a no-op to satisfy GameObserver.
func (NoopGameObserver) RecordWSMessage(string, time.Duration) {}

// SetOutboundQueueDepth is a no-op to satisfy GameObserver.
func (NoopGameObserver) SetOutboundQueueDepth(string, int) {}

// IncWSMessageDropped is a no-op to satisfy GameObserver.
func (NoopGameObserver) IncWSMessageDropped(string) {}

// IncPersistDropped is a no-op to satisfy GameObserver.
func (NoopGameObserver) IncPersistDropped() {}

// SetPersistLag is a no-op to satisfy GameObserver.
func (NoopGameObserver) SetPersistLag(string, time.Duration) {}

// IncGameResultMarshalFailures is a no-op to satisfy GameObserver.
func (NoopGameObserver) IncGameResultMarshalFailures() {}

// IncNicknameConfirm is a no-op to satisfy GameObserver.
func (NoopGameObserver) IncNicknameConfirm(bool) {}

// AuditRoomCreate is a no-op to satisfy GameObserver.
func (NoopGameObserver) AuditRoomCreate(context.Context, string, int) {}

// AuditRoomDelete is a no-op to satisfy GameObserver.
func (NoopGameObserver) AuditRoomDelete(context.Context, string) {}

// Observer returns the game observer. Returns NoopGameObserver when unset.
func (h *Hub) Observer() GameObserver {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.observer == nil {
		return NoopGameObserver{}
	}
	return h.observer
}

// SetObserver replaces the game observer. nil is ignored.
func (h *Hub) SetObserver(o GameObserver) {
	if o == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.observer = o
}
