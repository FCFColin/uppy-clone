package domain

// NicknameValidator validates nickname strings.
type NicknameValidator interface {
	ValidateNickname(nickname string) string
}
