package game

import (
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/nicknames"
	"github.com/uppy-clone/backend/internal/validate"
)

const roomAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// GenerateRoomCode 生成 config.RoomCodeLen 字符房间码
func GenerateRoomCode(rng RNGSource) string {
	code := make([]byte, config.RoomCodeLen)
	for i := range code {
		code[i] = roomAlphabet[rng.IntN(len(roomAlphabet))]
	}
	return string(code)
}

const maxNicknameLength = 12

// GenerateRandomNickname 从名字池随机组合生成昵称
func GenerateRandomNickname(usedNames map[string]bool) string {
	return nicknames.GenerateRandom(usedNames)
}

// GenerateUniqueNickname 生成不重复的随机昵称
func GenerateUniqueNickname(clientName string, usedNames map[string]bool) string {
	if clientName != "" {
		if validate.NicknameInputRejected(clientName) {
			return GenerateRandomNickname(usedNames)
		}
		truncated := clientName
		runeSlice := []rune(truncated)
		if len(runeSlice) > maxNicknameLength {
			truncated = string(runeSlice[:maxNicknameLength])
		}
		if truncated != "" && !usedNames[truncated] {
			return truncated
		}
	}
	return GenerateRandomNickname(usedNames)
}

// SanitizePlayerName 清理玩家名字：去除 XSS 向量、限制长度、折叠空白
func SanitizePlayerName(raw string) string {
	return validate.Nickname(raw)
}
