package nicknames

import (
	"strings"
	"testing"
)

func TestGenerateRandom_NonEmpty(t *testing.T) {
	for i := 0; i < 50; i++ {
		name := GenerateRandom(map[string]bool{})
		if name == "" {
			t.Fatal("GenerateRandom returned empty string")
		}
	}
}

func TestGenerateRandom_UniqueWhenUnused(t *testing.T) {
	used := map[string]bool{}
	for i := 0; i < 20; i++ {
		name := GenerateRandom(used)
		if used[name] {
			t.Fatalf("duplicate name %q on fresh map", name)
		}
		used[name] = true
	}
}

func TestGenerateRandom_AvoidsUsedBaseName(t *testing.T) {
	used := map[string]bool{"快乐的气球": true}
	for i := 0; i < 30; i++ {
		name := GenerateRandom(used)
		if name == "快乐的气球" {
			t.Fatal("should not return name in usedNames")
		}
	}
}

func TestGenerateRandom_FallbackSuffixOrPlayer(t *testing.T) {
	allBase := map[string]bool{}
	for _, adj := range NicknameAdjectives {
		for _, cat := range NicknameCategories {
			for _, noun := range cat {
				allBase[adj+noun] = true
			}
		}
	}

	name := GenerateRandom(allBase)
	if allBase[name] {
		t.Fatalf("returned used base name %q", name)
	}
	hasHash := strings.Contains(name, "#")
	hasPlayer := strings.HasPrefix(name, "Player")
	if !hasHash && !hasPlayer {
		t.Fatalf("expected # suffix or Player prefix fallback, got %q", name)
	}
}

func TestGenerateRandom_NilUsedNames(t *testing.T) {
	name := GenerateRandom(nil)
	if name == "" {
		t.Fatal("expected non-empty nickname for nil usedNames")
	}
}

func TestRandomIndex_NonPositive(t *testing.T) {
	if got := randomIndex(0); got != 0 {
		t.Errorf("randomIndex(0) = %d, want 0", got)
	}
	if got := randomIndex(-1); got != 0 {
		t.Errorf("randomIndex(-1) = %d, want 0", got)
	}
}
