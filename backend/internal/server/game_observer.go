package server

import (
	"context"
	"time"

	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/metrics"
)

// auditKeyCode is the audit map key for the room code field.
const auditKeyCode = "code"

// ProductionGameObserver implements game.GameObserver by delegating to the
// audit and metrics packages. It is wired into Hub via SetObserver during
// server initialization (server_init.go).
type ProductionGameObserver struct{}

// ── Room lifecycle ──────────────────────────────────────────────────

// SetActiveRooms updates the gauge of rooms currently active on this instance.
func (ProductionGameObserver) SetActiveRooms(count int) {
	metrics.ActiveRooms.Set(float64(count))
}

// IncGameSessions increments the counter of completed game sessions.
func (ProductionGameObserver) IncGameSessions() {
	metrics.GameSessionsTotal.Inc()
}

// ── Performance ─────────────────────────────────────────────────────

// RecordRoomLockHold records the duration a room lock was held, tagged by reason.
func (ProductionGameObserver) RecordRoomLockHold(reason string, d time.Duration) {
	metrics.RecordRoomLockHold(reason, d)
}

// RecordGameTick records the duration of a single game tick.
func (ProductionGameObserver) RecordGameTick(d time.Duration) {
	metrics.RecordGameTickDuration(d)
}

// RecordWSMessage records the duration of processing a WebSocket message, tagged by message name.
func (ProductionGameObserver) RecordWSMessage(msgName string, d time.Duration) {
	metrics.RecordWSMessage(msgName, d)
}

// ── Queue / buffer ──────────────────────────────────────────────────

// SetOutboundQueueDepth sets the current outbound queue depth gauge for a room.
func (ProductionGameObserver) SetOutboundQueueDepth(lobbyCode string, depth int) {
	metrics.SetRoomOutboundQueueDepth(lobbyCode, depth)
}

// IncWSMessageDropped increments the counter of WebSocket messages dropped
// before delivery, tagged by lobby code.
func (ProductionGameObserver) IncWSMessageDropped(lobbyCode string) {
	metrics.WSMessagesDroppedTotal.WithLabelValues(lobbyCode).Inc()
}

// IncPersistDropped increments the counter of room state persist attempts dropped.
func (ProductionGameObserver) IncPersistDropped() {
	metrics.RoomPersistDropped.Inc()
}

// ── Persist ─────────────────────────────────────────────────────────

// SetPersistLag sets the gauge of time spent persisting room state for a room.
func (ProductionGameObserver) SetPersistLag(code string, d time.Duration) {
	metrics.SetRoomPersistLag(code, d)
}

// IncGameResultMarshalFailures increments the counter of game result marshalling failures.
func (ProductionGameObserver) IncGameResultMarshalFailures() {
	metrics.GameResultMarshalFailures.Inc()
}

// ── Business ────────────────────────────────────────────────────────

// IncNicknameConfirm increments the counter of nickname confirmation outcomes,
// tagged by accepted/rejected.
func (ProductionGameObserver) IncNicknameConfirm(accepted bool) {
	label := "rejected"
	if accepted {
		label = "accepted"
	}
	metrics.NicknameConfirmTotal.WithLabelValues(label).Inc()
}

// ── Audit ───────────────────────────────────────────────────────────

// AuditRoomCreate writes an audit log entry for room creation, capturing the
// room code and max players.
func (ProductionGameObserver) AuditRoomCreate(ctx context.Context, code string, maxPlayers int) {
	audit.Log(ctx, audit.AuditEntry{
		Action:    "room.create",
		ActorType: audit.ActorTypeSystem,
		ActorID:   "system",
		Resource:  "room/" + code,
		After:     map[string]interface{}{auditKeyCode: code, "max_players": maxPlayers},
	})
}

// AuditRoomDelete writes an audit log entry for room deletion, capturing the
// room code as the resource being removed.
func (ProductionGameObserver) AuditRoomDelete(ctx context.Context, code string) {
	audit.Log(ctx, audit.AuditEntry{
		Action:    "room.delete",
		ActorType: audit.ActorTypeSystem,
		ActorID:   "system",
		Resource:  "room/" + code,
		Before:    map[string]interface{}{auditKeyCode: code},
	})
}

// Compile-time interface conformance check.
var _ game.GameObserver = ProductionGameObserver{}
