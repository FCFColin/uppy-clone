package domain

import (
	"strings"
	"testing"
	"unicode/utf8"
)

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

func TestNicknameValidatorFunc(t *testing.T) {
	fn := NicknameValidatorFunc(func(_ string) string {
		return "fixed"
	})
	if got := fn.ValidateNickname("anything"); got != "fixed" {
		t.Errorf("ValidateNickname = %q, want %q", got, "fixed")
	}
}

func TestDefaultValidator(t *testing.T) {
	got := DefaultValidator.ValidateNickname("<script>alert(1)</script>")
	if len(got) >= len("<script>alert(1)</script>") {
		t.Errorf("DefaultValidator should strip HTML: got %q", got)
	}
	if got != SanitizeNickname("<script>alert(1)</script>") {
		t.Errorf("DefaultValidator result should match SanitizeNickname: got %q, SanitizeNickname gave %q", got, SanitizeNickname("<script>alert(1)</script>"))
	}
}

func TestSanitizeNickname_ControlStripping(t *testing.T) {
	got := SanitizeNickname("hello\nworld")
	if got != "helloworld" {
		t.Errorf("SanitizeNickname with newline = %q, want %q", got, "helloworld")
	}

	got = SanitizeNickname("tab\there")
	if got != "tabhere" {
		t.Errorf("SanitizeNickname with tab = %q, want %q", got, "tabhere")
	}

	got = SanitizeNickname("\r\n")
	if got != "" {
		t.Errorf("SanitizeNickname with only CRLF = %q, want empty", got)
	}
}
