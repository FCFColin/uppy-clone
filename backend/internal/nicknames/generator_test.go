package nicknames

import (
	"errors"
	"io"
	"math/big"
	"strconv"
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

func TestGenerateRandom_UsesHashSuffixWhenBaseTaken(t *testing.T) {
	used := map[string]bool{}
	for _, adj := range NicknameAdjectives {
		for _, cat := range NicknameCategories {
			for _, noun := range cat {
				used[adj+noun] = true
			}
		}
	}
	for i := 0; i < 200; i++ {
		name := GenerateRandom(used)
		if strings.Contains(name, "#") {
			return
		}
		if strings.HasPrefix(name, "Player") {
			return
		}
	}
	t.Fatal("expected # suffix or Player fallback when all base names taken")
}

func TestGenerateRandom_PlayerFallback(t *testing.T) {
	used := map[string]bool{}
	for _, adj := range NicknameAdjectives {
		for _, cat := range NicknameCategories {
			for _, noun := range cat {
				base := adj + noun
				used[base] = true
				for i := 2; i < 100; i++ {
					candidate := base + "#" + strconv.Itoa(i)
					if len(candidate) <= maxNicknameLength {
						used[candidate] = true
					}
				}
			}
		}
	}

	name := GenerateRandom(used)
	if !strings.HasPrefix(name, "Player") {
		t.Fatalf("expected Player fallback, got %q", name)
	}
}

func TestGenerateRandom_SkipsOverlongSuffixCandidates(t *testing.T) {
	used := map[string]bool{}
	for _, adj := range NicknameAdjectives {
		for _, cat := range NicknameCategories {
			for _, noun := range cat {
				base := adj + noun
				used[base] = true
				for i := 2; i < 100; i++ {
					candidate := base + "#" + strconv.Itoa(i)
					if len(candidate) <= maxNicknameLength {
						used[candidate] = true
					}
				}
			}
		}
	}
	name := GenerateRandom(used)
	if !strings.HasPrefix(name, "Player") {
		t.Fatalf("expected Player fallback after long suffix skips, got %q", name)
	}
}

func TestRandomIndex_RandFailure(t *testing.T) {
	defer SetRandIntHook(func(_ io.Reader, _ *big.Int) (*big.Int, error) {
		return nil, errors.New("rand failed")
	})()

	if got := randomIndex(10); got != 0 {
		t.Errorf("randomIndex on rand failure = %d, want 0", got)
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

func TestGenerateRandom_ReturnsValidSuffix(t *testing.T) {
	prev := randIntFn
	randIntFn = func(_ io.Reader, max *big.Int) (*big.Int, error) {
		return big.NewInt(0), nil
	}
	t.Cleanup(func() { randIntFn = prev })

	adj := NicknameAdjectives[0]
	noun := NicknameCategories[0][0]
	base := adj + noun
	used := map[string]bool{base: true}
	want := base + "#2"
	if len(want) > maxNicknameLength {
		t.Skip("base name too long for suffix test")
	}
	name := GenerateRandom(used)
	if name != want {
		t.Fatalf("GenerateRandom() = %q, want %q", name, want)
	}
}

func TestGenerateRandom_SkipsLongSuffixCandidate(t *testing.T) {
	defer SetRandIntHook(func(_ io.Reader, max *big.Int) (*big.Int, error) {
		return big.NewInt(0), nil
	})()

	adj := NicknameAdjectives[0]
	cat := NicknameCategories[0]
	noun := cat[0]
	base := adj + noun
	used := map[string]bool{base: true}
	for i := 2; i < 100; i++ {
		candidate := base + "#" + strconv.Itoa(i)
		if len(candidate) > maxNicknameLength {
			used[candidate] = true
		}
	}
	name := GenerateRandom(used)
	if strings.HasPrefix(name, base+"#") && len(name) > maxNicknameLength {
		t.Fatalf("should skip overlong suffix candidate, got %q", name)
	}
}
