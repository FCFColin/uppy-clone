package game

import (
	"strings"
	"testing"
)

func TestGenerateRandomNickname_NonEmpty(t *testing.T) {
	for _, excluded := range []map[string]bool{map[string]bool{}, nil} {
		for i := 0; i < 100; i++ {
			name := GenerateRandomNickname(excluded)
			if name == "" {
				t.Fatal("生成的昵称不应为空")
			}
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
	allNames := getAllNicknameCombinations()
	excludeAll := make(map[string]bool)
	for _, n := range allNames {
		excludeAll[n] = true
	}

	name := GenerateRandomNickname(excludeAll)

	if excludeAll[name] {
		t.Fatalf("返回的名字不应在排除列表中，got=%s", name)
	}
	hasHash := strings.Contains(name, "#")
	hasPlayer := strings.HasPrefix(name, "Player")
	if !hasHash && !hasPlayer {
		t.Fatalf("排除所有基础组合后应返回带 # 后缀或 Player 前缀的名字，got=%s", name)
	}
}

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

func TestGenerateUniqueNickname_Truncation(t *testing.T) {
	usedNames := map[string]bool{}
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

func TestSanitizePlayerName(t *testing.T) {
	if got := SanitizePlayerName("  hello  "); got != "hello" {
		t.Fatalf("SanitizePlayerName = %q", got)
	}
}
