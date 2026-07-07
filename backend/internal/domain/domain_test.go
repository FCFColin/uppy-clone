package domain

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

type testValidator struct{}

func (testValidator) ValidateNickname(s string) string { return s }

var _testValidator = testValidator{}

func TestNewNickname_Valid(t *testing.T) {
	n, err := NewNickname("  Hello World  ", _testValidator)
	if err != nil {
		t.Fatalf("NewNickname: %v", err)
	}
	// truncated to MaxNicknameLen (12) runes
	if n.String() != "  Hello Worl" {
		t.Errorf("got %q", n.String())
	}
}

func TestNewNickname_Empty(t *testing.T) {
	if _, err := NewNickname("", _testValidator); err == nil {
		t.Fatal("expected empty nickname error")
	}
}

func TestNewNickname_StripsDangerousChars(t *testing.T) {
	// Identity validator preserves all input, truncated to MaxNicknameLen
	n, err := NewNickname("<script>alert</script>", _testValidator)
	if err != nil {
		t.Fatalf("NewNickname: %v", err)
	}
	if n.String() != "<script>aler" {
		t.Errorf("got %q", n.String())
	}
}

func TestNewNickname_TruncatesToTwelveRunes(t *testing.T) {
	long := strings.Repeat("字", 20)
	n, err := NewNickname(long, _testValidator)
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(n.String())) != 12 {
		t.Errorf("expected 12 runes, got %d", len([]rune(n.String())))
	}
}

func TestNewRoomCode_Valid(t *testing.T) {
	code, err := NewRoomCode("ABCD2")
	if err != nil {
		t.Fatalf("NewRoomCode: %v", err)
	}
	if code.String() != "ABCD2" {
		t.Errorf("got %q", code.String())
	}
}

func TestNewRoomCode_InvalidLength(t *testing.T) {
	if _, err := NewRoomCode("ABC"); err == nil {
		t.Fatal("expected length error")
	}
}

func TestNewRoomCode_InvalidChar(t *testing.T) {
	if _, err := NewRoomCode("ABCD0"); err == nil {
		t.Fatal("expected invalid char error for 0")
	}
}

func TestEventTypes(t *testing.T) {
	now := time.Now()
	events := []Event{
		PlayerJoined{DomainEvent: DomainEvent{At: now}, RoomCode: "ABCD2"},
		PlayerLeft{DomainEvent: DomainEvent{At: now}, RoomCode: "ABCD2"},
		GameEnded{DomainEvent: DomainEvent{At: now}, RoomCode: "ABCD2"},
		PhaseChanged{DomainEvent: DomainEvent{At: now}, RoomCode: "ABCD2"},
		UserHardDeleted{DomainEvent: DomainEvent{At: now}, UserID: "u1"},
	}
	for _, e := range events {
		if e.EventType() == "" || e.OccurredAt().IsZero() {
			t.Errorf("invalid event %T", e)
		}
	}
}

func TestUserHardDeletedEvent(t *testing.T) {
	now := time.Now()
	e := UserHardDeleted{DomainEvent: DomainEvent{At: now}, UserID: "u1"}
	if et := e.EventType(); et != "user.hard_deleted" {
		t.Errorf("EventType = %q, want %q", et, "user.hard_deleted")
	}
	if at := e.OccurredAt(); at != now {
		t.Errorf("OccurredAt = %v, want %v", at, now)
	}
}

func TestContextKey_WithValue(t *testing.T) {
	ctx := context.Background()
	ctx = ContextKeyUserID.WithValue(ctx, "user123")
	val, ok := ContextKeyUserID.Value(ctx)
	if !ok {
		t.Error("Value should return ok=true for set key")
	}
	if val != "user123" {
		t.Errorf("Value = %q, want %q", val, "user123")
	}
}

func TestContextKey_Value_NotFound(t *testing.T) {
	ctx := context.Background()
	_, ok := ContextKeyUserID.Value(ctx)
	if ok {
		t.Error("Value should return ok=false for unset key")
	}
}

func TestContextKey_WrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), ContextKeyUserID, 42)
	_, ok := ContextKeyUserID.Value(ctx)
	if ok {
		t.Error("Value should return ok=false for wrong value type")
	}
}

func TestContextKey_MultipleKeys(t *testing.T) {
	ctx := context.Background()
	ctx = ContextKeyUserID.WithValue(ctx, "u1")
	ctx = ContextKeyNickname.WithValue(ctx, "nick1")
	ctx = ContextKeyRole.WithValue(ctx, "admin")
	ctx = ContextKeyJTI.WithValue(ctx, "jti1")

	val, ok := ContextKeyUserID.Value(ctx)
	if !ok || val != "u1" {
		t.Errorf("ContextKeyUserID = %q, ok=%v", val, ok)
	}
	val, ok = ContextKeyNickname.Value(ctx)
	if !ok || val != "nick1" {
		t.Errorf("ContextKeyNickname = %q, ok=%v", val, ok)
	}
	val, ok = ContextKeyRole.Value(ctx)
	if !ok || val != "admin" {
		t.Errorf("ContextKeyRole = %q, ok=%v", val, ok)
	}
	val, ok = ContextKeyJTI.Value(ctx)
	if !ok || val != "jti1" {
		t.Errorf("ContextKeyJTI = %q, ok=%v", val, ok)
	}
}

func TestUnmarshalRoomRegistryInfo(t *testing.T) {
	data := []byte(`{"code":"ABCD2","instance":"i1","address":"addr","created_at":1000}`)
	info, err := UnmarshalRoomRegistryInfo(data)
	if err != nil {
		t.Fatalf("UnmarshalRoomRegistryInfo: %v", err)
	}
	if info.Code != "ABCD2" {
		t.Errorf("Code = %q, want %q", info.Code, "ABCD2")
	}
	if info.Instance != "i1" {
		t.Errorf("Instance = %q, want %q", info.Instance, "i1")
	}
	if info.Address != "addr" {
		t.Errorf("Address = %q, want %q", info.Address, "addr")
	}
	if info.CreatedAt != 1000 {
		t.Errorf("CreatedAt = %d, want %d", info.CreatedAt, 1000)
	}
}

func TestUnmarshalRoomRegistryInfo_InvalidJSON(t *testing.T) {
	_, err := UnmarshalRoomRegistryInfo([]byte(`{invalid}`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestGameState_SerializeDeserialize(t *testing.T) {
	original := &GameState{
		Players: map[string]*PlayerState{
			"p1": {ID: "p1", ScoreContribution: 100},
		},
		Phase: PhasePlaying,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var restored GameState
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if restored.Phase != original.Phase {
		t.Errorf("Phase mismatch: %v vs %v", restored.Phase, original.Phase)
	}
	if !reflect.DeepEqual(restored.Players["p1"], original.Players["p1"]) {
		t.Errorf("Players mismatch: %+v vs %+v", restored.Players["p1"], original.Players["p1"])
	}
}

func TestGameState_BadJSON(t *testing.T) {
	var gs GameState
	if err := json.Unmarshal([]byte(`{bad json}`), &gs); err == nil {
		t.Error("expected unmarshal error")
	}
}
