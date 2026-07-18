package domain

// NicknameValidator validates nickname strings.
type NicknameValidator interface {
	ValidateNickname(nickname string) string
}

// NicknameValidatorFunc adapts a function to NicknameValidator.
type NicknameValidatorFunc func(string) string

// ValidateNickname delegates to the underlying function to validate a nickname.
func (f NicknameValidatorFunc) ValidateNickname(nickname string) string {
	return f(nickname)
}

// DefaultValidator is a ready-to-use NicknameValidator.
var DefaultValidator NicknameValidator = NicknameValidatorFunc(SanitizeNickname)
