// Package validate provides input validation utilities.
package validate

import (
	"regexp"
	"strings"
)

// 常量与正则 — 企业为何需要：统一昵称清理逻辑消除 game/names.go 与 auth/jwt.go 的重复实现，
// 集中维护 XSS 防护与字符过滤规则。
const maxNicknameLength = 20

var (
	controlCharsRegex   = regexp.MustCompile(`[\x00-\x1F\x7F-\x9F]`)
	zeroWidthCharsRegex = regexp.MustCompile(`[\x{200B}-\x{200F}\x{FEFF}\x{2028}-\x{202F}\x{2060}-\x{206F}]`)
	htmlCharsRegex      = regexp.MustCompile(`[<>"'\x60&]`)
	whitespaceRegex     = regexp.MustCompile(`\s+`)
)

// Nickname sanitizes a player nickname.
// Removes control characters, zero-width chars, HTML special chars,
// trims whitespace, limits length, and collapses whitespace.
// 企业为何需要：XSS 防护与字符过滤规则必须集中管理，避免多处实现漂移导致安全漏洞。
func Nickname(raw string) string {
	if raw == "" {
		return ""
	}
	// 控制字符 U+0000-U+001F, U+007F-U+009F
	raw = controlCharsRegex.ReplaceAllString(raw, "")
	// 零宽字符 U+200B-U+200F, U+FEFF 及其他不可见 Unicode
	raw = zeroWidthCharsRegex.ReplaceAllString(raw, "")
	raw = strings.TrimSpace(raw)
	// 截断到 20 字符
	runeSlice := []rune(raw)
	if len(runeSlice) > maxNicknameLength {
		raw = string(runeSlice[:maxNicknameLength])
	}
	// HTML 特殊字符
	raw = htmlCharsRegex.ReplaceAllString(raw, "")
	// 折叠空白
	raw = whitespaceRegex.ReplaceAllString(raw, " ")
	return raw
}
