package domain

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// --- RoomCode validation ---
//
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

// --- Nickname sanitization ---

func TestSanitizeNickname_EmptyString(t *testing.T) {
	if got := SanitizeNickname(""); got != "" {
		t.Errorf("SanitizeNickname(\"\") = %q, want empty string", got)
	}
}

func TestSanitizeNickname_ValidNames(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple ASCII", "Player1", "Player1"},
		{"CJK characters", "你好世界", "你好世界"},
		{"mixed CJK and ASCII", "Player玩家", "Player玩家"},
		{"single rune emoji", "🎮gamer", "🎮gamer"},
		{"single char", "A", "A"},
		{"numbers only", "12345", "12345"},
		{"spaces inside", "hello world", "hello world"},
		{"exactly max runes", strings.Repeat("1", MaxNicknameLen), strings.Repeat("1", MaxNicknameLen)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeNickname(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeNickname(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeNickname_ControlChars(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"NULL byte", "hel\x00lo", "hello"},
		{"TAB removed", "a\tb", "ab"},
		{"only control chars", "\x00\x01\x02\x7F\u009F", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeNickname(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeNickname(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeNickname_ZeroWidthChars(t *testing.T) {
	got := SanitizeNickname("he\u200Bllo")
	if got != "hello" {
		t.Errorf("SanitizeNickname = %q, want hello", got)
	}
}

func TestSanitizeNickname_HTMLSpecialChars(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"a<b", "ab"},
		{"a&b", "ab"},
		{`a"b`, "ab"},
	}
	for _, tt := range tests {
		got := SanitizeNickname(tt.input)
		if got != tt.want {
			t.Errorf("SanitizeNickname(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSanitizeNickname_XSSAttacks(t *testing.T) {
	inputs := []string{
		"<script>alert(1)</script>",
		"<img src=x onerror=alert(1)>",
		`"'<>` + "`" + `&`,
	}
	dangerous := []string{"<", ">", `"`, "'", "`", "&"}
	for _, input := range inputs {
		got := SanitizeNickname(input)
		for _, d := range dangerous {
			if strings.Contains(got, d) {
				t.Errorf("SECURITY: SanitizeNickname(%q) = %q contains %q", input, got, d)
			}
		}
	}
}

func TestSanitizeNickname_LengthTruncation(t *testing.T) {
	long := strings.Repeat("a", 30)
	got := SanitizeNickname(long)
	if utf8.RuneCountInString(got) > MaxNicknameLen {
		t.Errorf("output has %d runes, max %d", utf8.RuneCountInString(got), MaxNicknameLen)
	}
}

func TestSanitizeNickname_CJKTruncation(t *testing.T) {
	input := strings.Repeat("你", MaxNicknameLen+1)
	got := SanitizeNickname(input)
	if utf8.RuneCountInString(got) != MaxNicknameLen {
		t.Errorf("rune count = %d, want %d", utf8.RuneCountInString(got), MaxNicknameLen)
	}
}

func TestSanitizeNickname_WhitespaceHandling(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"  hello  ", "hello"},
		{"hello   world", "hello world"},
		{"   ", ""},
	}
	for _, tt := range tests {
		got := SanitizeNickname(tt.input)
		if got != tt.want {
			t.Errorf("SanitizeNickname(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- Nickname rejection ---

func TestNicknameInputRejected(t *testing.T) {
	if !NicknameInputRejected("<script>") {
		t.Fatal("expected script tag to be rejected")
	}
	if NicknameInputRejected("正常的昵称") {
		t.Fatal("expected valid CJK nickname to be accepted")
	}
	if !NicknameInputRejected("hello\x00world") {
		t.Fatal("expected control char to be rejected")
	}
}
