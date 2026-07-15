package validate

import (
	"strings"
	"testing"
	"unicode/utf8"

	"pgregory.net/rapid"
)

// TestNickname_PropertyOutputRuneCountWithinLimit: For any input string,
// the sanitized output never exceeds maxNicknameLen runes.
func TestNickname_PropertyOutputRuneCountWithinLimit(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		raw := rapid.String().Draw(t, "raw")
		got := Nickname(raw)
		if utf8.RuneCountInString(got) > maxNicknameLen {
			t.Fatalf("output has %d runes, max %d (input=%q)", utf8.RuneCountInString(got), maxNicknameLen, raw)
		}
	})
}

// TestNickname_PropertyOutputHasNoControlChars: The output never contains
// ASCII control characters (U+0000–U+001F, U+007F–U+009F).
func TestNickname_PropertyOutputHasNoControlChars(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		raw := rapid.String().Draw(t, "raw")
		got := Nickname(raw)
		for _, r := range got {
			if r < 0x20 || (r >= 0x7F && r <= 0x9F) {
				t.Fatalf("output contains control char U+%04X (input=%q, output=%q)", r, raw, got)
			}
		}
	})
}

// TestNickname_PropertyOutputHasNoHTMLSpecialChars: The output never contains
// HTML-special characters (< > " ' ` &).
func TestNickname_PropertyOutputHasNoHTMLSpecialChars(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		raw := rapid.String().Draw(t, "raw")
		got := Nickname(raw)
		dangerous := []string{"<", ">", `"`, "'", "`", "&"}
		for _, d := range dangerous {
			if strings.Contains(got, d) {
				t.Fatalf("output contains HTML char %q (input=%q, output=%q)", d, raw, got)
			}
		}
	})
}

// TestNickname_PropertyOutputHasNoInvisibleChars: The output never contains
// zero-width / invisible characters (U+200B–U+200F, U+FEFF, U+2028–U+202F, U+2060–U+206F).
func TestNickname_PropertyOutputHasNoInvisibleChars(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		raw := rapid.String().Draw(t, "raw")
		got := Nickname(raw)
		for _, r := range got {
			if (r >= 0x200B && r <= 0x200F) || r == 0xFEFF ||
				(r >= 0x2028 && r <= 0x202F) || (r >= 0x2060 && r <= 0x206F) {
				t.Fatalf("output contains invisible char U+%04X (input=%q, output=%q)", r, raw, got)
			}
		}
	})
}

// TestNickname_PropertyEmptyInputReturnsEmpty: An empty input always yields an empty output.
func TestNickname_PropertyEmptyInputReturnsEmpty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		if got := Nickname(""); got != "" {
			t.Fatalf("Nickname(\"\") = %q, want empty", got)
		}
	})
}
