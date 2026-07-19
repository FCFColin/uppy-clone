package game

import (
	"context"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/config"
)

// NoopGameObserver 是 Hub 的零值默认观察者——必须满足 GameObserver 全部方法，
// 否则 Hub.observer 字段赋值会编译失败。它本身没有行为，但接口契约必须冻结。

func TestNoopGameObserver_SatisfiesGameObserver(t *testing.T) {
	t.Parallel()
	// Compile-time interface satisfaction. If GameObserver gains a method,
	// this line fails to compile, forcing authors to add the no-op.
	var _ GameObserver = NoopGameObserver{}
	var _ GameObserver = (*NoopGameObserver)(nil)
}

// recordingObserver is a test double that captures every call so Hub tests
// can assert observer wiring without depending on Prometheus.
type recordingObserver struct {
	activeRooms      int
	gameSessions     int
	lockHoldReasons  []string
	tickDurations    []time.Duration
	wsMessageNames   []string
	queueDepths      map[string]int
	wsDroppedRooms   []string
	persistDropped   int
	persistLags      map[string]time.Duration
	marshalFailures  int
	nicknameAccepted int
	nicknameRejected int
	auditRoomCreates []string
	auditRoomDeletes []string
}

func newRecordingObserver() *recordingObserver {
	return &recordingObserver{
		queueDepths: make(map[string]int),
		persistLags: make(map[string]time.Duration),
	}
}

func (r *recordingObserver) SetActiveRooms(n int) { r.activeRooms = n }

func (r *recordingObserver) IncGameSessions() { r.gameSessions++ }

func (r *recordingObserver) RecordRoomLockHold(reason string, _ time.Duration) {
	r.lockHoldReasons = append(r.lockHoldReasons, reason)
}

func (r *recordingObserver) RecordGameTick(d time.Duration) {
	r.tickDurations = append(r.tickDurations, d)
}

func (r *recordingObserver) RecordWSMessage(name string, _ time.Duration) {
	r.wsMessageNames = append(r.wsMessageNames, name)
}

func (r *recordingObserver) SetOutboundQueueDepth(code string, depth int) {
	r.queueDepths[code] = depth
}

func (r *recordingObserver) IncWSMessageDropped(code string) {
	r.wsDroppedRooms = append(r.wsDroppedRooms, code)
}

func (r *recordingObserver) IncPersistDropped() { r.persistDropped++ }

func (r *recordingObserver) SetPersistLag(code string, d time.Duration) {
	r.persistLags[code] = d
}

func (r *recordingObserver) IncGameResultMarshalFailures() { r.marshalFailures++ }

func (r *recordingObserver) IncNicknameConfirm(accepted bool) {
	if accepted {
		r.nicknameAccepted++
	} else {
		r.nicknameRejected++
	}
}

func (r *recordingObserver) AuditRoomCreate(_ context.Context, code string, _ int) {
	r.auditRoomCreates = append(r.auditRoomCreates, code)
}

func (r *recordingObserver) AuditRoomDelete(_ context.Context, code string) {
	r.auditRoomDeletes = append(r.auditRoomDeletes, code)
}

func TestRecordingObserver_SatisfiesGameObserver(t *testing.T) {
	t.Parallel()
	// Compile-time check that recordingObserver implements GameObserver.
	// Used by other game tests; if the interface grows, this catches it.
	var _ GameObserver = (*recordingObserver)(nil)
}

func TestRecordingObserver_CapturesCalls(t *testing.T) {
	t.Parallel()
	o := newRecordingObserver()

	o.SetActiveRooms(3)
	o.IncGameSessions()
	o.IncGameSessions()
	o.RecordRoomLockHold("create", 5*time.Millisecond)
	o.RecordGameTick(2 * time.Millisecond)
	o.RecordWSMessage("tap", 1*time.Millisecond)
	o.SetOutboundQueueDepth("ABCDE", 7)
	o.IncWSMessageDropped("ABCDE")
	o.IncPersistDropped()
	o.SetPersistLag("ABCDE", 50*time.Millisecond)
	o.IncGameResultMarshalFailures()
	o.IncNicknameConfirm(true)
	o.IncNicknameConfirm(false)
	o.AuditRoomCreate(context.Background(), "ABCDE", 4)
	o.AuditRoomDelete(context.Background(), "ABCDE")

	if o.activeRooms != 3 {
		t.Fatalf("activeRooms = %d, want 3", o.activeRooms)
	}
	if o.gameSessions != 2 {
		t.Fatalf("gameSessions = %d, want 2", o.gameSessions)
	}
	if len(o.lockHoldReasons) != 1 || o.lockHoldReasons[0] != "create" {
		t.Fatalf("lockHoldReasons = %v, want [create]", o.lockHoldReasons)
	}
	if len(o.tickDurations) != 1 {
		t.Fatalf("tickDurations len = %d, want 1", len(o.tickDurations))
	}
	if len(o.wsMessageNames) != 1 || o.wsMessageNames[0] != "tap" {
		t.Fatalf("wsMessageNames = %v, want [tap]", o.wsMessageNames)
	}
	if o.queueDepths["ABCDE"] != 7 {
		t.Fatalf("queueDepths[ABCDE] = %d, want 7", o.queueDepths["ABCDE"])
	}
	if len(o.wsDroppedRooms) != 1 || o.wsDroppedRooms[0] != "ABCDE" {
		t.Fatalf("wsDroppedRooms = %v, want [ABCDE]", o.wsDroppedRooms)
	}
	if o.persistDropped != 1 {
		t.Fatalf("persistDropped = %d, want 1", o.persistDropped)
	}
	if o.persistLags["ABCDE"] != 50*time.Millisecond {
		t.Fatalf("persistLags[ABCDE] = %v, want 50ms", o.persistLags["ABCDE"])
	}
	if o.marshalFailures != 1 {
		t.Fatalf("marshalFailures = %d, want 1", o.marshalFailures)
	}
	if o.nicknameAccepted != 1 || o.nicknameRejected != 1 {
		t.Fatalf("nickname accepted/rejected = %d/%d, want 1/1", o.nicknameAccepted, o.nicknameRejected)
	}
	if len(o.auditRoomCreates) != 1 || o.auditRoomCreates[0] != "ABCDE" {
		t.Fatalf("auditRoomCreates = %v, want [ABCDE]", o.auditRoomCreates)
	}
	if len(o.auditRoomDeletes) != 1 || o.auditRoomDeletes[0] != "ABCDE" {
		t.Fatalf("auditRoomDeletes = %v, want [ABCDE]", o.auditRoomDeletes)
	}
}

func TestHub_SetObserver_ReplacesDefault(t *testing.T) {
	t.Parallel()
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0)

	// Default observer is NoopGameObserver.
	if _, ok := h.Observer().(NoopGameObserver); !ok {
		t.Fatalf("default observer = %T, want NoopGameObserver", h.Observer())
	}

	o := newRecordingObserver()
	h.SetObserver(o)
	if h.Observer() != o {
		t.Fatalf("Observer() = %p, want %p (recordingObserver)", h.Observer(), o)
	}
}

func TestHub_SetObserver_NilIgnored(t *testing.T) {
	t.Parallel()
	h := NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0)

	o := newRecordingObserver()
	h.SetObserver(o)
	h.SetObserver(nil) // should be ignored

	if h.Observer() != o {
		t.Fatal("SetObserver(nil) should not replace the existing observer")
	}
}
