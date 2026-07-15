package nicknames

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestGenerateRandom_PropertyNeverEmpty: For any usedNames set (including nil and
// populated maps), GenerateRandom always returns a non-empty string.
func TestGenerateRandom_PropertyNeverEmpty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 50).Draw(t, "n")
		usedNames := make(map[string]bool, n)
		for i := 0; i < n; i++ {
			usedNames[rapid.String().Draw(t, "key")] = true
		}
		name := GenerateRandom(usedNames)
		if name == "" {
			t.Fatal("GenerateRandom returned empty string")
		}
	})
}

// TestGenerateRandom_PropertyEmptyUsedNamesReturnsBaseCombo: With an empty usedNames
// map, the first random combo is always returned directly — no '#'-suffix and no
// 'Player' fallback, because the candidate is never in the empty set.
func TestGenerateRandom_PropertyEmptyUsedNamesReturnsBaseCombo(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		name := GenerateRandom(map[string]bool{})
		if name == "" {
			t.Fatal("expected non-empty name")
		}
		if strings.Contains(name, "#") {
			t.Fatalf("expected base combo without # suffix, got %q", name)
		}
		if strings.HasPrefix(name, "Player") {
			t.Fatalf("expected base combo without Player fallback, got %q", name)
		}
	})
}

// TestGenerateRandom_PropertyAllBaseUsedTriggersFallback: When every base combo is
// already in usedNames, the result must use either a '#' suffix or the Player fallback.
func TestGenerateRandom_PropertyAllBaseUsedTriggersFallback(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		allBase := make(map[string]bool)
		for _, adj := range NicknameAdjectives {
			for _, cat := range NicknameCategories {
				for _, noun := range cat {
					allBase[adj+noun] = true
				}
			}
		}
		name := GenerateRandom(allBase)
		if allBase[name] {
			t.Fatalf("returned a used base name %q", name)
		}
		if !strings.Contains(name, "#") && !strings.HasPrefix(name, "Player") {
			t.Fatalf("expected # suffix or Player fallback, got %q", name)
		}
	})
}

// TestGenerateRandom_PropertyAccumulatingDistinct: Generating multiple nicknames with
// an accumulating usedNames set always produces names not already in the set.
func TestGenerateRandom_PropertyAccumulatingDistinct(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		usedNames := make(map[string]bool)
		count := rapid.IntRange(1, 20).Draw(t, "count")
		for i := 0; i < count; i++ {
			name := GenerateRandom(usedNames)
			if usedNames[name] {
				t.Fatalf("iteration %d: name %q already in usedNames", i, name)
			}
			usedNames[name] = true
		}
	})
}
