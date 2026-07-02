package domain

import (
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
	if n.String() != "  Hello World  " {
		t.Errorf("got %q", n.String())
	}
}

func TestNewNickname_Empty(t *testing.T) {
	if _, err := NewNickname("", _testValidator); err == nil {
		t.Fatal("expected empty nickname error")
	}
}

func TestNewNickname_StripsDangerousChars(t *testing.T) {
	// Identity validator preserves all input
	n, err := NewNickname("<script>alert</script>", _testValidator)
	if err != nil {
		t.Fatalf("NewNickname: %v", err)
	}
	if n.String() != "<script>alert</script>" {
		t.Errorf("got %q", n.String())
	}
}

func TestNewNickname_TruncatesToTwelveRunes(t *testing.T) {
	long := strings.Repeat("字", 20)
	n, err := NewNickname(long, _testValidator)
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(n.String())) != 20 {
		t.Errorf("len = %d", len([]rune(n.String())))
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
		PlayerJoined{RoomCode: "ABCD2", At: now},
		PlayerLeft{RoomCode: "ABCD2", At: now},
		GameEnded{RoomCode: "ABCD2", At: now},
		PhaseChanged{RoomCode: "ABCD2", At: now},
	}
	for _, e := range events {
		if e.EventType() == "" || e.OccurredAt().IsZero() {
			t.Errorf("invalid event %T", e)
		}
	}
}
