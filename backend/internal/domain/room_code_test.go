package domain

import (
	"strings"
	"testing"
)

// NewRoomCode 验证规则：
// - 长度必须为 5
// - 字符必须在 [A-Z2-9] 范围内（即 A-Z 或 2-9）
// - 注意：实现实际上接受 I 和 O（它们在 [A-Z] 范围内），尽管源码注释说
//   "去除易混淆的 0/1/I/O"——这是注释与实现不一致的已知问题（见报告）。
//   生成器侧 (roomAlphabet) 才真正排除 I/O/0/1。

func TestNewRoomCode_ValidBoundaries(t *testing.T) {
	t.Parallel()
	// Lowercase should be rejected.
	if _, err := NewRoomCode("abcde"); err == nil {
		t.Fatal("expected error for lowercase letters")
	}
	// All digits 2-9 valid.
	if _, err := NewRoomCode("23456"); err != nil {
		t.Fatalf("expected valid for 23456, got: %v", err)
	}
	// Zero and One should be rejected (out of [A-Z2-9]).
	if _, err := NewRoomCode("ABC10"); err == nil {
		t.Fatal("expected error for chars 1 and 0")
	}
	// Digits 0 alone rejected.
	if _, err := NewRoomCode("ABCD0"); err == nil {
		t.Fatal("expected error for char 0")
	}
	// Digit 1 alone rejected.
	if _, err := NewRoomCode("ABCD1"); err == nil {
		t.Fatal("expected error for char 1")
	}
}

func TestNewRoomCode_AcceptsIAndO(t *testing.T) {
	t.Parallel()
	// Documents a known discrepancy: the source comment says I/O are excluded
	// ("去除易混淆的 0/1/I/O") but the implementation's range check
	// `(c < 'A' || c > 'Z') && (c < '2' || c > '9')` admits I and O because
	// they fall in [A-Z]. This test pins the current behavior so the
	// discrepancy is visible. To actually exclude I/O, the impl would need
	// an explicit denylist like the generator's roomAlphabet.
	if _, err := NewRoomCode("ABCDI"); err != nil {
		t.Fatalf("NewRoomCode(ABCDI) returned error %v; current impl accepts I (see comment in room_code.go)", err)
	}
	if _, err := NewRoomCode("ABCDO"); err != nil {
		t.Fatalf("NewRoomCode(ABCDO) returned error %v; current impl accepts O (see comment in room_code.go)", err)
	}
}

func TestNewRoomCode_LengthBoundaries(t *testing.T) {
	t.Parallel()
	if _, err := NewRoomCode(""); err == nil {
		t.Fatal("expected error for empty code")
	}
	if _, err := NewRoomCode("ABCD"); err == nil {
		t.Fatal("expected error for 4-char code")
	}
	if _, err := NewRoomCode("ABCDEF"); err == nil {
		t.Fatal("expected error for 6-char code")
	}
	if _, err := NewRoomCode("ABCDE"); err != nil {
		t.Fatalf("expected valid for ABCDE, got: %v", err)
	}
}

func TestNewRoomCode_SpecialCharsRejected(t *testing.T) {
	t.Parallel()
	invalidCodes := []string{
		"ABC!D",
		"ABC-D",
		"ABC D",
		"ABC.D",
		"ABC*D",
		"abcde",
		"ABCDé",
	}
	for _, code := range invalidCodes {
		if _, err := NewRoomCode(code); err == nil {
			t.Fatalf("expected error for code %q", code)
		}
	}
}

func TestNewRoomCode_AcceptsAllValidChars(t *testing.T) {
	t.Parallel()
	// Verify every char in [A-Z] and [2-9] is accepted (in a 5-char code).
	for c := byte('A'); c <= 'Z'; c++ {
		code := string([]byte{'A', 'B', 'C', 'D', c})
		if _, err := NewRoomCode(code); err != nil {
			t.Fatalf("NewRoomCode(%q) returned error: %v", code, err)
		}
	}
	for c := byte('2'); c <= '9'; c++ {
		code := string([]byte{'A', 'B', 'C', 'D', c})
		if _, err := NewRoomCode(code); err != nil {
			t.Fatalf("NewRoomCode(%q) returned error: %v", code, err)
		}
	}
}

func TestNewRoomCode_String(t *testing.T) {
	t.Parallel()
	c, err := NewRoomCode("XYZ23")
	if err != nil {
		t.Fatalf("NewRoomCode: %v", err)
	}
	if c.String() != "XYZ23" {
		t.Fatalf("String() = %q, want %q", c.String(), "XYZ23")
	}
}

func TestRoomCode_StringEmpty(t *testing.T) {
	t.Parallel()
	var c RoomCode
	if c.String() != "" {
		t.Fatalf("empty RoomCode String() = %q, want empty", c.String())
	}
}

func TestRoomCode_ErrorMessagesContainChar(t *testing.T) {
	t.Parallel()
	_, err := NewRoomCode("ABC1D")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "1") {
		t.Fatalf("error message should mention invalid char '1', got: %v", err)
	}
}

func TestRoomCode_ErrorMessagesContainLength(t *testing.T) {
	t.Parallel()
	_, err := NewRoomCode("ABC")
	if err == nil {
		t.Fatal("expected error for short code")
	}
	if !strings.Contains(err.Error(), "3") {
		t.Fatalf("error message should mention length 3, got: %v", err)
	}
}
