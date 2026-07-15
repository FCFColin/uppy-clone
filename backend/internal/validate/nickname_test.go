package validate

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNickname_EmptyString(t *testing.T) {
	if got := Nickname(""); got != "" {
		t.Errorf("Nickname(\"\") = %q, want empty string", got)
	}
}

func TestNickname_ValidNames(t *testing.T) {
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
		{"exactly max runes", strings.Repeat("1", maxNicknameLen), strings.Repeat("1", maxNicknameLen)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Nickname(tt.input)
			if got != tt.want {
				t.Errorf("Nickname(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNickname_ControlChars(t *testing.T) {
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
			got := Nickname(tt.input)
			if got != tt.want {
				t.Errorf("Nickname(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNickname_ZeroWidthChars(t *testing.T) {
	got := Nickname("he\u200Bllo")
	if got != "hello" {
		t.Errorf("Nickname = %q, want hello", got)
	}
}

func TestNickname_HTMLSpecialChars(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"a<b", "ab"},
		{"a&b", "ab"},
		{`a"b`, "ab"},
	}
	for _, tt := range tests {
		got := Nickname(tt.input)
		if got != tt.want {
			t.Errorf("Nickname(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNickname_XSSAttacks(t *testing.T) {
	inputs := []string{
		"<script>alert(1)</script>",
		"<img src=x onerror=alert(1)>",
		`"'<>` + "`" + `&`,
	}
	dangerous := []string{"<", ">", `"`, "'", "`", "&"}
	for _, input := range inputs {
		got := Nickname(input)
		for _, d := range dangerous {
			if strings.Contains(got, d) {
				t.Errorf("SECURITY: Nickname(%q) = %q contains %q", input, got, d)
			}
		}
	}
}

func TestNickname_LengthTruncation(t *testing.T) {
	long := strings.Repeat("a", 30)
	got := Nickname(long)
	if utf8.RuneCountInString(got) > maxNicknameLen {
		t.Errorf("output has %d runes, max %d", utf8.RuneCountInString(got), maxNicknameLen)
	}
}

func TestNickname_CJKTruncation(t *testing.T) {
	input := strings.Repeat("你", maxNicknameLen+1)
	got := Nickname(input)
	if utf8.RuneCountInString(got) != maxNicknameLen {
		t.Errorf("rune count = %d, want %d", utf8.RuneCountInString(got), maxNicknameLen)
	}
}

func TestNickname_WhitespaceHandling(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"  hello  ", "hello"},
		{"hello   world", "hello world"},
		{"   ", ""},
	}
	for _, tt := range tests {
		got := Nickname(tt.input)
		if got != tt.want {
			t.Errorf("Nickname(%q) = %q, want %q", tt.input, got, tt.want)
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
	if got != Nickname("<script>alert(1)</script>") {
		t.Errorf("DefaultValidator result should match Nickname: got %q, Nickname gave %q", got, Nickname("<script>alert(1)</script>"))
	}
}

func TestNickname_ControlStripping(t *testing.T) {
	got := Nickname("hello\nworld")
	if got != "helloworld" {
		t.Errorf("Nickname with newline = %q, want %q", got, "helloworld")
	}

	got = Nickname("tab\there")
	if got != "tabhere" {
		t.Errorf("Nickname with tab = %q, want %q", got, "tabhere")
	}

	got = Nickname("\r\n")
	if got != "" {
		t.Errorf("Nickname with only CRLF = %q, want empty", got)
	}
}
