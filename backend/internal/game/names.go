package game

import (
	"crypto/rand"
	"math/big"
	"regexp"
	"strconv"

	"github.com/uppy-clone/backend/internal/validate"
)

var dangerousCharsRegex = regexp.MustCompile(`[\x00-\x1f<>"'&]`)

var NicknameAdjectives = []string{
	"快乐的", "勇敢的", "神秘的", "聪明的", "幸运的", "飞翔的", "闪耀的", "敏捷的",
	"温柔的", "狂野的", "优雅的", "调皮的", "冷静的", "热情的", "沉默的", "活泼的",
	"机智的", "憨厚的", "灵巧的", "威武的", "慵懒的", "专注的", "飘逸的", "坚定的",
	"好奇的", "悠闲的", "霸气的", "呆萌的", "睿智的", "潇洒的", "顽强的",
	"璀璨的", "朦胧的", "炽热的", "冰冷的", "迅捷的", "沉稳的", "天真的", "深沉的",
	"从容的", "执着的", "豪迈的",
	"灵动的", "梦幻的", "寂静的", "奔放的", "细腻的", "豪放的", "清新的", "绚烂的",
	"悠然的", "坚韧的", "开朗的", "内敛的", "浪漫的", "朴实的", "华丽的", "素雅的",
}

// NicknameAnimals is the list of animals for random nickname generation.
var NicknameAnimals = []string{
	"气球", "老鹰", "海豚", "狐狸", "熊猫", "猫咪", "小鹿", "飞鸟",
	"鲸鱼", "蝴蝶", "松鼠", "兔子", "猫头鹰", "企鹅", "海龟", "萤火虫",
	"刺猬", "海鸥", "燕子", "知更鸟", "独角兽", "龙猫",
	"小熊", "浣熊", "灰狼", "雪豹",
}

// NicknameJobs is the list of jobs for random nickname generation.
var NicknameJobs = []string{
	"探险家", "飞行员", "冒险者", "梦想家", "旅行者", "守护者", "追逐者", "航海家",
	"工程师", "艺术家", "音乐家", "诗人", "骑士", "游侠", "法师", "炼金师",
	"天文学家", "园丁", "面包师", "钟表匠", "摄影师", "收藏家",
	"建筑师", "工匠", "猎人", "学者",
}

var NicknameNature = []string{
	"星辰", "月光", "微风", "晨露", "晚霞", "彩虹", "雪花", "阳光",
	"云朵", "海浪", "山峦", "森林", "溪流", "花朵", "落叶", "流星",
	"极光", "春雨", "夏日", "秋风", "冬雪", "朝雾",
	"星空", "银河", "潮汐", "晨曦",
}

// NicknameScifi is the list of sci-fi words for random nickname generation.
var NicknameScifi = []string{
	"量子", "光子", "星舰", "虫洞", "星云", "黑洞", "彗星", "卫星",
	"反应堆", "引擎", "芯片", "代码", "像素", "数据", "信号", "频段",
	"轨道", "空间站", "传送门", "力场", "激光", "等离子",
	"中子", "超新星", "陨石", "星尘",
}

// NicknameCategories 包含所有名词类别（与 TS NICKNAME_CATEGORIES 对应）
var NicknameCategories = [][]string{
	NicknameAnimals,
	NicknameJobs,
	NicknameNature,
	NicknameScifi,
}

const maxNicknameLength = 12

// randomIndex 返回 [0, n) 的随机整数
func randomIndex(n int) int {
	if n <= 0 {
		return 0
	}
	bigN := big.NewInt(int64(n))
	r, err := rand.Int(rand.Reader, bigN)
	if err != nil {
		// fallback — 不应发生
		return 0
	}
	return int(r.Int64())
}

// GenerateRandomNickname 从名字池随机组合生成昵称
//
// 流程：
// 1. 从名字池随机组合，最多重试 10 次
// 2. 仍重复则加数字后缀（如 "敏捷的飞行员#2"）
// 3. 兜底返回 PlayerXXXX
func GenerateRandomNickname(usedNames map[string]bool) string {
	// 1. 从名字池随机组合，最多重试 10 次
	for i := 0; i < 10; i++ {
		adj := NicknameAdjectives[randomIndex(len(NicknameAdjectives))]
		cat := NicknameCategories[randomIndex(len(NicknameCategories))]
		noun := cat[randomIndex(len(cat))]
		candidate := adj + noun
		if !usedNames[candidate] {
			return candidate
		}
	}

	// 2. 仍重复则加数字后缀
	adj := NicknameAdjectives[randomIndex(len(NicknameAdjectives))]
	cat := NicknameCategories[randomIndex(len(NicknameCategories))]
	noun := cat[randomIndex(len(cat))]
	baseName := adj + noun
	for i := 2; i < 100; i++ {
		candidate := baseName + "#" + strconv.Itoa(i)
		if len(candidate) <= maxNicknameLength && !usedNames[candidate] {
			return candidate
		}
	}

	// 3. 兜底
	return "Player" + strconv.Itoa(randomIndex(10000))
}

// GenerateUniqueNickname 生成不重复的随机昵称
//
// 流程：
// 1. 若客户端提供 name 且通过安全检查（无危险字符）且未重复，直接使用（超长则截断）
// 2. 否则调用 GenerateRandomNickname 从名字池生成
func GenerateUniqueNickname(clientName string, usedNames map[string]bool) string {
	// 1. 客户端提供的非空名字
	if clientName != "" {
		// 拒绝包含危险字符的名字（XSS 防护）
		if dangerousCharsRegex.MatchString(clientName) {
			return GenerateRandomNickname(usedNames)
		}
		// 截断到最大长度
		truncated := clientName
		runeSlice := []rune(truncated)
		if len(runeSlice) > maxNicknameLength {
			truncated = string(runeSlice[:maxNicknameLength])
		}
		if truncated != "" && !usedNames[truncated] {
			return truncated
		}
	}

	// 2-3. 调用 GenerateRandomNickname 从名字池生成
	return GenerateRandomNickname(usedNames)
}

// SanitizePlayerName 清理玩家名字：去除 XSS 向量、限制长度、折叠空白
// 委托给 validate.Nickname 统一实现，消除重复代码。
func SanitizePlayerName(raw string) string {
	return validate.Nickname(raw)
}
