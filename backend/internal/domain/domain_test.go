package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var _testValidator = func(s string) string { return s }

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

// --- Role context helpers ---

func TestWithRole_InjectsRole(t *testing.T) {
	t.Parallel()
	ctx := WithRole(context.Background(), RoleAdmin)
	role, ok := RoleFromContext(ctx)
	if !ok {
		t.Fatal("RoleFromContext should return ok=true after WithRole")
	}
	if role != RoleAdmin {
		t.Fatalf("role = %q, want %q", role, RoleAdmin)
	}
}

func TestRoleFromContext_EmptyContext(t *testing.T) {
	t.Parallel()
	role, ok := RoleFromContext(context.Background())
	if ok {
		t.Fatal("RoleFromContext should return ok=false on empty context")
	}
	if role != "" {
		t.Fatalf("role = %q, want empty", role)
	}
}

func TestWithRole_OverridesPreviousRole(t *testing.T) {
	t.Parallel()
	ctx := WithRole(context.Background(), RoleUser)
	ctx = WithRole(ctx, RoleAdmin)
	role, _ := RoleFromContext(ctx)
	if role != RoleAdmin {
		t.Fatalf("role = %q, want %q", role, RoleAdmin)
	}
}

func TestWithRole_PreservesOtherContextValues(t *testing.T) {
	t.Parallel()
	ctx := ContextKeyUserID.WithValue(context.Background(), "user-1")
	ctx = WithRole(ctx, RoleUser)

	userID, ok := ContextKeyUserID.Value(ctx)
	if !ok || userID != "user-1" {
		t.Fatalf("userID lost after WithRole: %q (ok=%v)", userID, ok)
	}
	role, ok := RoleFromContext(ctx)
	if !ok || role != RoleUser {
		t.Fatalf("role = %q (ok=%v)", role, ok)
	}
}

// --- ProblemDetails / RFC7807 ---

func TestProblemDetails_Constructors(t *testing.T) {
	tests := []struct {
		name   string
		pd     *ProblemDetails
		status int
		title  string
	}{
		{"BadRequest", BadRequest("bad"), http.StatusBadRequest, "Bad Request"},
		{"Unauthorized", Unauthorized("no auth"), http.StatusUnauthorized, "Unauthorized"},
		{"Forbidden", Forbidden("denied"), http.StatusForbidden, "Forbidden"},
		{"NotFound", NotFound("missing"), http.StatusNotFound, "Not Found"},
		{"Conflict", Conflict("dup"), http.StatusConflict, "Conflict"},
		{"UnprocessableEntity", UnprocessableEntity("invalid"), http.StatusUnprocessableEntity, "Unprocessable Entity"},
		{"TooManyRequests", TooManyRequests("slow down"), http.StatusTooManyRequests, "Too Many Requests"},
		{"InternalError", InternalError("boom"), http.StatusInternalServerError, "Internal Server Error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.pd.Status != tt.status || tt.pd.Title != tt.title {
				t.Fatalf("status/title mismatch: %+v", tt.pd)
			}
			wantType := fmt.Sprintf("https://httpstatuses.com/%d", tt.status)
			if tt.pd.Type != wantType {
				t.Errorf("type = %q, want %q", tt.pd.Type, wantType)
			}
		})
	}
}

func TestProblemDetails_Write(t *testing.T) {
	// Adversarial: malformed client must receive RFC7807 JSON, not HTML error page.
	pd := New(http.StatusForbidden, "Forbidden", "X-User-Role header spoofing denied")
	rec := httptest.NewRecorder()
	pd.Write(rec)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var got ProblemDetails
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Detail != pd.Detail || got.Status != pd.Status {
		t.Errorf("body mismatch: %+v", got)
	}
}

// --- PlayerState / GameState behavior ---

func TestPlayerCanTap(t *testing.T) {
	t.Parallel()
	now := time.Now().UnixMilli()

	t.Run("can tap when cooldown expired", func(t *testing.T) {
		p := &PlayerState{CooldownEndTime: now - 1}
		if !p.CanTap(now) {
			t.Error("CanTap should return true when cooldown has expired")
		}
	})

	t.Run("cannot tap during cooldown", func(t *testing.T) {
		p := &PlayerState{CooldownEndTime: now + 10000}
		if p.CanTap(now) {
			t.Error("CanTap should return false during cooldown")
		}
	})

	t.Run("can tap when cooldown equals now", func(t *testing.T) {
		p := &PlayerState{CooldownEndTime: now}
		if !p.CanTap(now) {
			t.Error("CanTap should return true when cooldown equals now")
		}
	})

	t.Run("can tap with zero cooldown", func(t *testing.T) {
		p := &PlayerState{CooldownEndTime: 0}
		if !p.CanTap(now) {
			t.Error("CanTap should return true with zero cooldown")
		}
	})
}

func TestPlayerRecordTap(t *testing.T) {
	t.Parallel()
	now := time.Now().UnixMilli()

	p := &PlayerState{CooldownEndTime: 0, TapsCount: 0, ScoreContribution: 0}
	p.RecordTap(now, 5000)

	if p.CooldownEndTime != now+5000 {
		t.Errorf("CooldownEndTime = %d, want %d", p.CooldownEndTime, now+5000)
	}
	if p.TapsCount != 1 {
		t.Errorf("TapsCount = %d, want 1", p.TapsCount)
	}
	if p.ScoreContribution != 1 {
		t.Errorf("ScoreContribution = %d, want 1", p.ScoreContribution)
	}
}

func TestPlayerRecordTap_Multiple(t *testing.T) {
	t.Parallel()
	p := &PlayerState{}
	for i := 0; i < 5; i++ {
		p.RecordTap(1000, 100)
	}
	if p.TapsCount != 5 {
		t.Errorf("TapsCount = %d, want 5", p.TapsCount)
	}
	if p.ScoreContribution != 5 {
		t.Errorf("ScoreContribution = %d, want 5", p.ScoreContribution)
	}
}

func TestPlayerMarkDisconnected(t *testing.T) {
	t.Parallel()
	now := time.Now().UnixMilli()

	p := &PlayerState{Disconnected: false, DisconnectedAt: nil}
	p.MarkDisconnected(now)

	if !p.Disconnected {
		t.Error("Disconnected should be true after MarkDisconnected")
	}
	if p.DisconnectedAt == nil {
		t.Fatal("DisconnectedAt should not be nil")
	}
	if *p.DisconnectedAt != now {
		t.Errorf("DisconnectedAt = %d, want %d", *p.DisconnectedAt, now)
	}
}

func TestGameStateIsGameOver(t *testing.T) {
	t.Parallel()

	t.Run("ended phase returns true", func(t *testing.T) {
		gs := &GameState{Phase: PhaseEnded}
		if !gs.IsGameOver() {
			t.Error("IsGameOver should return true for PhaseEnded")
		}
	})

	t.Run("waiting phase returns false", func(t *testing.T) {
		gs := &GameState{Phase: PhaseWaiting}
		if gs.IsGameOver() {
			t.Error("IsGameOver should return false for PhaseWaiting")
		}
	})

	t.Run("countdown phase returns false", func(t *testing.T) {
		gs := &GameState{Phase: PhaseCountdown}
		if gs.IsGameOver() {
			t.Error("IsGameOver should return false for PhaseCountdown")
		}
	})

	t.Run("playing phase returns false", func(t *testing.T) {
		gs := &GameState{Phase: PhasePlaying}
		if gs.IsGameOver() {
			t.Error("IsGameOver should return false for PhasePlaying")
		}
	})
}
