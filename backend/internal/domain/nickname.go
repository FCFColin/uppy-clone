package domain

import (
	"fmt"

	"github.com/uppy-clone/backend/internal/validate"
)

// Nickname is a value object representing a sanitized player nickname.
type Nickname string

// NewNickname creates a Nickname from raw input using validate.Nickname rules.
func NewNickname(name string) (Nickname, error) {
	sanitized := validate.Nickname(name)
	if sanitized == "" {
		return "", fmt.Errorf("nickname cannot be empty")
	}
	return Nickname(sanitized), nil
}

// String returns the string representation.
func (n Nickname) String() string {
	return string(n)
}
