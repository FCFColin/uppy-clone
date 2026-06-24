package game

import (
	"strings"
	"testing"
)

// ─── GenerateRandomNickname ──────────────────────────────────────────

func TestGenerateRandomNickname_NonEmpty(t *testing.T) {
	for i := 0; i < 100; i++ {
		name := GenerateRandomNickname(map[string]bool{})
		if name == "" {
			t.Fatal("生成的昵称不应为空")
		}
	}
}

func TestGenerateRandomNickname_ContainsDe(t *testing.T) {
	for i := 0; i < 50; i++ {
		name := GenerateRandomNickname(map[string]bool{})
		if !strings.Contains(name, "的") {
			t.Fatalf("生成的昵称应包含'的'字, got=%s", name)
		}
	}
}

func TestGenerateRandomNickname_ExcludeList(t *testing.T) {
	excluded := map[string]bool{"敏捷的飞行员": true}
	for i := 0; i < 50; i++ {
		name := GenerateRandomNickname(excluded)
		if name == "敏捷的飞行员" {
			t.Fatal("排除列表中的名字不应被生成")
		}
	}
}

func TestGenerateRandomNickname_AllExcluded(t *testing.T) {
	// 排除所有基础组合 → 应返回带 # 后缀的名字或 PlayerXXXX 兜底
	allNames := getAllNicknameCombinations()
	excludeAll := make(map[string]bool)
	for _, n := range allNames {
		excludeAll[n] = true
	}

	name := GenerateRandomNickname(excludeAll)

	if excludeAll[name] {
		t.Fatalf("返回的名字不应在排除列表中，got=%s", name)
	}
	// 应返回带 # 后缀的名字或 PlayerXXXX 兜底
	hasHash := strings.Contains(name, "#")
	hasPlayer := strings.HasPrefix(name, "Player")
	if !hasHash && !hasPlayer {
		t.Fatalf("排除所有基础组合后应返回带 # 后缀或 Player 前缀的名字，got=%s", name)
	}
}

func TestGenerateRandomNickname_EmptyExclude(t *testing.T) {
	name := GenerateRandomNickname(nil)
	if name == "" {
		t.Fatal("nil 排除列表时应正常生成")
	}
}

// ─── GenerateUniqueNickname ──────────────────────────────────────────

func TestGenerateUniqueNickname_NoConflict(t *testing.T) {
	usedNames := map[string]bool{"已占用": true}
	result := GenerateUniqueNickname("玩家甲", usedNames)
	if result != "玩家甲" {
		t.Fatalf("不重复的名字应直接使用，got=%s", result)
	}
}

func TestGenerateUniqueNickname_Conflict(t *testing.T) {
	usedNames := map[string]bool{"玩家甲": true}
	result := GenerateUniqueNickname("玩家甲", usedNames)
	if result == "玩家甲" {
		t.Fatal("重复的名字应生成随机名字")
	}
	if usedNames[result] {
		t.Fatal("生成的名字不应在已用列表中")
	}
}

func TestGenerateUniqueNickname_Empty(t *testing.T) {
	usedNames := map[string]bool{}
	result := GenerateUniqueNickname("", usedNames)
	if len(result) == 0 {
		t.Fatal("空名字应生成随机昵称")
	}
}

func TestGenerateUniqueNickname_DangerousChars(t *testing.T) {
	usedNames := map[string]bool{}
	result := GenerateUniqueNickname("<script>alert(1)</script>", usedNames)
	if strings.Contains(result, "<") || strings.Contains(result, ">") {
		t.Fatalf("危险字符名字应被拒绝并生成随机昵称，got=%s", result)
	}
}

// ─── SanitizePlayerName ──────────────────────────────────────────────

func TestSanitizePlayerName_Normal(t *testing.T) {
	result := SanitizePlayerName("正常名字")
	if result != "正常名字" {
		t.Fatalf("正常名字应原样返回，got=%s", result)
	}
}

func TestSanitizePlayerName_ScriptTag(t *testing.T) {
	result := SanitizePlayerName("<script>")
	if strings.Contains(result, "<") || strings.Contains(result, ">") {
		t.Fatalf("HTML 特殊字符应被移除，got=%s", result)
	}
}

func TestSanitizePlayerName_Truncate(t *testing.T) {
	long := strings.Repeat("a", 1000)
	result := SanitizePlayerName(long)
	if len([]rune(result)) != 20 {
		t.Fatalf("超长名字应截断至 20 字符，got len=%d", len([]rune(result)))
	}
}

func TestSanitizePlayerName_Empty(t *testing.T) {
	result := SanitizePlayerName("")
	if result != "" {
		t.Fatalf("空字符串应返回空字符串，got=%s", result)
	}
}

func TestSanitizePlayerName_Whitespace(t *testing.T) {
	result := SanitizePlayerName("  多余  空格  ")
	if result != "多余 空格" {
		t.Fatalf("多余空白应折叠为单个空格，got=%s", result)
	}
}

func TestSanitizePlayerName_Quotes(t *testing.T) {
	result := SanitizePlayerName("a'b\"c")
	if result != "abc" {
		t.Fatalf("引号字符应被去除，got=%s", result)
	}
}

func TestSanitizePlayerName_Ampersand(t *testing.T) {
	result := SanitizePlayerName("a&b")
	if result != "ab" {
		t.Fatalf("& 符号应被去除，got=%s", result)
	}
}

func TestSanitizePlayerName_OnlyDangerous(t *testing.T) {
	result := SanitizePlayerName("<>\"'`&")
	if result != "" {
		t.Fatalf("仅含危险字符应返回空字符串，got=%s", result)
	}
}

func TestSanitizePlayerName_ControlChars(t *testing.T) {
	result := SanitizePlayerName("hello\x00world\x1F")
	if result != "helloworld" {
		t.Fatalf("控制字符应被移除，got=%s", result)
	}
}

// ─── 内部辅助：获取所有昵称组合 ─────────────────────────────────────

func getAllNicknameCombinations() []string {
	var names []string
	for _, adj := range NicknameAdjectives {
		for _, cat := range NicknameCategories {
			for _, noun := range cat {
				names = append(names, adj+noun)
			}
		}
	}
	return names
}

// ─── randomIndex ─────────────────────────────────────────────────────

func TestRandomIndex(t *testing.T) {
	// Valid range
	for i := 0; i < 50; i++ {
		idx := randomIndex(10)
		if idx < 0 || idx >= 10 {
			t.Fatalf("randomIndex(10) = %d, want [0, 10)", idx)
		}
	}

	// Zero or negative
	idx := randomIndex(0)
	if idx != 0 {
		t.Fatalf("randomIndex(0) = %d, want 0", idx)
	}
	idx = randomIndex(-1)
	if idx != 0 {
		t.Fatalf("randomIndex(-1) = %d, want 0", idx)
	}
}

// ─── trimSpace ───────────────────────────────────────────────────────

func TestTrimSpace(t *testing.T) {
	if trimSpace("  hello  ") != "hello" {
		t.Fatal("trimSpace should trim spaces")
	}
	if trimSpace("hello") != "hello" {
		t.Fatal("trimSpace should not modify already trimmed strings")
	}
}

// ─── GenerateUniqueNickname edge cases ───────────────────────────────

func TestGenerateUniqueNickname_Truncation(t *testing.T) {
	usedNames := map[string]bool{}
	// Very long name should be truncated
	longName := strings.Repeat("a", 50)
	result := GenerateUniqueNickname(longName, usedNames)
	if len([]rune(result)) > 12 {
		t.Fatalf("long client name should be truncated to 12 chars, got %d", len([]rune(result)))
	}
}

func TestGenerateUniqueNickname_ClientNameNotInUse(t *testing.T) {
	usedNames := map[string]bool{}
	result := GenerateUniqueNickname("Alice", usedNames)
	if result != "Alice" {
		t.Fatalf("unused client name should be used directly, got %q", result)
	}
}

func TestGenerateUniqueNickname_ClientNameInUse(t *testing.T) {
	usedNames := map[string]bool{"Alice": true}
	result := GenerateUniqueNickname("Alice", usedNames)
	if result == "Alice" {
		t.Fatal("used client name should trigger random generation")
	}
}
