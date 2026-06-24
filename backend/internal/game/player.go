package game

import (
	"time"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

// HandleSetNickname 处理设置昵称请求
//
// 包含 30 秒冷却（首次改名跳过冷却），防止频繁修改。
// 验证：长度字段越界检查、控制字符和 HTML 特殊字符过滤、空昵称忽略、
// 长度限制 12 字符、当前房间内重复检查。
func HandleSetNickname(_ *domain.GameState, player *domain.PlayerState, nickname string, usedNames map[string]bool) bool {
	now := time.Now().UnixMilli()

	// 首次改名（lastNicknameChange === 0）跳过冷却
	if player.LastNicknameChange != 0 && now-player.LastNicknameChange < protocol.NicknameCooldownMs {
		return false
	}

	// 内容过滤：去除控制字符、零宽字符和 HTML 特殊字符
	nickname = sanitizeNickname(nickname)
	if nickname == "" {
		return false
	}

	// 长度限制统一为 12 字符
	runeSlice := []rune(nickname)
	if len(runeSlice) > protocol.MaxNicknameLen {
		nickname = string(runeSlice[:protocol.MaxNicknameLen])
	}

	// 与当前昵称相同则无需修改
	if nickname == player.Nickname {
		return false
	}

	// 重复检查：若与 usedNames 重复，服务端重新生成不重复的名字
	if usedNames[nickname] {
		nickname = GenerateUniqueNickname(nickname, usedNames)
	}

	// 更新 usedNames：移除旧名、加入新名
	delete(usedNames, player.Nickname)
	usedNames[nickname] = true

	player.LastNicknameChange = now
	player.Nickname = nickname
	return true
}

// sanitizeNickname 清理昵称输入（对应 TS handleSetNickname 中的过滤逻辑）
func sanitizeNickname(raw string) string {
	// 控制字符 U+0000-U+001F, U+007F-U+009F
	re1 := controlCharsRegex.ReplaceAllString(raw, "")
	// 零宽字符
	re2 := zeroWidthCharsRegex.ReplaceAllString(re1, "")
	// HTML 特殊字符
	re3 := htmlCharsRegex.ReplaceAllString(re2, "")
	// trim
	result := trimSpace(re3)
	return result
}
