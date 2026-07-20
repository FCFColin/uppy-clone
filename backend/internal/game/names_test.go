package game

import (
	"strings"
	"testing"
)

func TestGenerateRandomNickname_TableDriven(t *testing.T) {
	allNames := getAllNicknameCombinations()
	excludeAll := make(map[string]bool)
	for _, n := range allNames {
		excludeAll[n] = true
	}

	tests := []struct {
		name             string
		excluded         map[string]bool
		testNilToo       bool // 同时测试 nil 和 excluded 两个 map
		iterations       int
		wantNonEmpty     bool
		wantContains     string
		wantNotEqual     string
		wantHashOrPlayer bool
	}{
		{
			name:         "NonEmpty",
			excluded:     map[string]bool{},
			testNilToo:   true,
			iterations:   100,
			wantNonEmpty: true,
		},
		{
			name:         "ContainsDe",
			excluded:     map[string]bool{},
			iterations:   50,
			wantContains: "的",
		},
		{
			name:         "ExcludeList",
			excluded:     map[string]bool{"敏捷的飞行员": true},
			iterations:   50,
			wantNotEqual: "敏捷的飞行员",
		},
		{
			name:             "AllExcluded",
			excluded:         excludeAll,
			iterations:       1,
			wantNonEmpty:     true,
			wantHashOrPlayer: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapsToTest := []map[string]bool{tt.excluded}
			if tt.testNilToo {
				mapsToTest = append(mapsToTest, nil)
			}
			for _, excluded := range mapsToTest {
				for i := 0; i < tt.iterations; i++ {
					name := GenerateRandomNickname(excluded)
					if tt.wantNonEmpty && name == "" {
						t.Fatal("生成的昵称不应为空")
					}
					if tt.wantContains != "" && !strings.Contains(name, tt.wantContains) {
						t.Fatalf("生成的昵称应包含%q, got=%s", tt.wantContains, name)
					}
					if tt.wantNotEqual != "" && name == tt.wantNotEqual {
						t.Fatalf("排除列表中的名字不应被生成, got=%s", name)
					}
					if tt.wantHashOrPlayer {
						if excludeAll[name] {
							t.Fatalf("返回的名字不应在排除列表中，got=%s", name)
						}
						hasHash := strings.Contains(name, "#")
						hasPlayer := strings.HasPrefix(name, "Player")
						if !hasHash && !hasPlayer {
							t.Fatalf("排除所有基础组合后应返回带 # 后缀或 Player 前缀的名字，got=%s", name)
						}
					}
				}
			}
		})
	}
}

func TestGenerateUniqueNickname_TableDriven(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		usedNames      map[string]bool
		wantExact      string
		wantNotEqual   string
		wantNotInUsed  bool
		wantNonEmpty   bool
		wantNotContain []string
		wantMaxRunes   int
	}{
		{name: "NoConflict", input: "玩家甲", usedNames: map[string]bool{"已占用": true}, wantExact: "玩家甲"},
		{name: "Conflict", input: "玩家甲", usedNames: map[string]bool{"玩家甲": true}, wantNotEqual: "玩家甲", wantNotInUsed: true},
		{name: "Empty", input: "", usedNames: map[string]bool{}, wantNonEmpty: true},
		{name: "DangerousChars", input: "<script>alert(1)</script>", usedNames: map[string]bool{}, wantNotContain: []string{"<", ">"}},
		{name: "Truncation", input: strings.Repeat("a", 50), usedNames: map[string]bool{}, wantMaxRunes: 12},
		{name: "ClientNameNotInUse", input: "Alice", usedNames: map[string]bool{}, wantExact: "Alice"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateUniqueNickname(tt.input, tt.usedNames)
			if tt.wantExact != "" && result != tt.wantExact {
				t.Fatalf("不重复的名字应直接使用，got=%s", result)
			}
			if tt.wantNotEqual != "" && result == tt.wantNotEqual {
				t.Fatalf("重复的名字应生成随机名字, got=%s", result)
			}
			if tt.wantNotInUsed && tt.usedNames[result] {
				t.Fatalf("生成的名字不应在已用列表中, got=%s", result)
			}
			if tt.wantNonEmpty && len(result) == 0 {
				t.Fatal("空名字应生成随机昵称")
			}
			for _, c := range tt.wantNotContain {
				if strings.Contains(result, c) {
					t.Fatalf("危险字符名字应被拒绝并生成随机昵称，got=%s", result)
				}
			}
			if tt.wantMaxRunes > 0 && len([]rune(result)) > tt.wantMaxRunes {
				t.Fatalf("long client name should be truncated to %d chars, got %d", tt.wantMaxRunes, len([]rune(result)))
			}
		})
	}
}

func TestSanitizePlayerName(t *testing.T) {
	if got := SanitizePlayerName("  hello  "); got != "hello" {
		t.Fatalf("SanitizePlayerName = %q", got)
	}
}
