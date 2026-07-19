package domain

// NicknameValidator validates nickname strings.
type NicknameValidator func(string) string

// DefaultValidator is a ready-to-use NicknameValidator.
var DefaultValidator NicknameValidator = SanitizeNickname
