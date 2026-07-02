package validate

import "github.com/uppy-clone/backend/internal/domain"

// Ensure implementation satisfies interface.
var _ domain.NicknameValidator = NicknameValidatorFunc(nil)

// NicknameValidatorFunc adapts a function to domain.NicknameValidator.
type NicknameValidatorFunc func(string) string

func (f NicknameValidatorFunc) ValidateNickname(nickname string) string {
	return f(nickname)
}

// DefaultValidator is a ready-to-use domain.NicknameValidator.
var DefaultValidator domain.NicknameValidator = NicknameValidatorFunc(Nickname)
