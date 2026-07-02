// Package validate provides input validation utilities.
package validate

import (
	"regexp"
	"strings"
)

const maxNicknameLen = 12

var (
	controlCharsRegex   = regexp.MustCompile(`[\x00-\x1F\x7F-\x9F]`)
	zeroWidthCharsRegex = regexp.MustCompile(`[\x{200B}-\x{200F}\x{FEFF}\x{2028}-\x{202F}\x{2060}-\x{206F}]`)
	htmlCharsRegex      = regexp.MustCompile(`[<>"'\x60&]`)
	whitespaceRegex     = regexp.MustCompile(`\s+`)
)

// NicknameInputRejected reports whether raw input contains characters that must not
// be accepted as a client-provided nickname (control/HTML chars). Matches legacy game
// dangerousCharsRegex behavior for GenerateUniqueNickname.
func NicknameInputRejected(raw string) bool {
	return nicknameInputRejectedRegex.MatchString(raw)
}

var nicknameInputRejectedRegex = regexp.MustCompile(`[\x00-\x1f<>"'&]`)

// Nickname sanitizes a player nickname.
// Removes control characters, zero-width chars, HTML special chars,
// trims whitespace, limits length to maxNicknameLen runes, and collapses whitespace.
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
	if len(runeSlice) > maxNicknameLen {
		raw = string(runeSlice[:maxNicknameLen])
	}
	return raw
}
