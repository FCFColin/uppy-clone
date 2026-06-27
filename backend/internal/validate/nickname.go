// Package validate provides input validation utilities.
package validate

import (
	"regexp"
	"strings"

	"github.com/uppy-clone/backend/internal/protocol"
)

var (
	controlCharsRegex   = regexp.MustCompile(`[\x00-\x1F\x7F-\x9F]`)
	zeroWidthCharsRegex = regexp.MustCompile(`[\x{200B}-\x{200F}\x{FEFF}\x{2028}-\x{202F}\x{2060}-\x{206F}]`)
	htmlCharsRegex      = regexp.MustCompile(`[<>"'\x60&]`)
	whitespaceRegex     = regexp.MustCompile(`\s+`)
)

// Nickname sanitizes a player nickname.
// Removes control characters, zero-width chars, HTML special chars,
// trims whitespace, limits length to protocol.MaxNicknameLen runes, and collapses whitespace.
func Nickname(raw string) string {
	if raw == "" {
		return ""
	}
	raw = controlCharsRegex.ReplaceAllString(raw, "")
	raw = zeroWidthCharsRegex.ReplaceAllString(raw, "")
	raw = strings.TrimSpace(raw)
	raw = htmlCharsRegex.ReplaceAllString(raw, "")
	raw = whitespaceRegex.ReplaceAllString(raw, " ")
	runeSlice := []rune(raw)
	if len(runeSlice) > protocol.MaxNicknameLen {
		raw = string(runeSlice[:protocol.MaxNicknameLen])
	}
	return raw
}
