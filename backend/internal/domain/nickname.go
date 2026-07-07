package domain

import "fmt"

// Nickname is a value object representing a sanitized player nickname.
type Nickname string

// NewNickname creates a Nickname from raw input using the provided validator.
func NewNickname(name string, v NicknameValidator) (Nickname, error) {
	sanitized := v.ValidateNickname(name)
	runes := []rune(sanitized)
	if len(runes) > MaxNicknameLen {
		runes = runes[:MaxNicknameLen]
	}
	if len(runes) == 0 {
		return "", fmt.Errorf("nickname cannot be empty")
	}
	return Nickname(string(runes)), nil
}

// String returns the string representation.
func (n Nickname) String() string {
	return string(n)
}
