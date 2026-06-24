package domain

import (
	"fmt"
	"strings"
	"unicode"
)

// Nickname is a value object representing a sanitized player nickname.
// 企业为何需要：Value Object 封装长度与字符过滤校验，确保昵称始终合法。
//
// TODO(P3-4.3): 未来新代码应使用 Nickname 替代裸 string 类型；
// 全量替换现有 string 类型风险过大，暂不进行。
type Nickname string

// NewNickname creates a Nickname, returning an error if invalid.
// 过滤规则：仅保留字母、数字、空格、下划线、连字符；最长 12 字符（按 rune 计）。
func NewNickname(name string) (Nickname, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("nickname cannot be empty")
	}
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	result := b.String()
	if runes := []rune(result); len(runes) > 12 {
		result = string(runes[:12])
	}
	return Nickname(result), nil
}

// String returns the string representation.
func (n Nickname) String() string {
	return string(n)
}
