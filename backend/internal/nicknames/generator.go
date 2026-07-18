// Package nicknames provides random nickname generation for game players.
package nicknames

import (
	"crypto/rand"
	"math/big"
	"strconv"
	"unicode/utf8"
)

const maxNicknameLength = 12

// maxSuffixAttempts limits the number of #N suffix tries before falling back to PlayerXXXX.
const maxSuffixAttempts = 98

func randomIndex(n int) int {
	if n <= 0 {
		return 0
	}
	bigN := big.NewInt(int64(n))
	r, err := randIntFn(rand.Reader, bigN)
	if err != nil {
		return 0
	}
	return int(r.Int64())
}

func pickRandomCombo() string {
	adj := NicknameAdjectives[randomIndex(len(NicknameAdjectives))]
	cat := NicknameCategories[randomIndex(len(NicknameCategories))]
	noun := cat[randomIndex(len(cat))]
	return adj + noun
}

// GenerateRandom produces a random nickname from the word pools.
// If all attempts collide with usedNames, appends #N suffix or falls back to PlayerXXXX.
func GenerateRandom(usedNames map[string]bool) string {
	for i := 0; i < 10; i++ {
		candidate := pickRandomCombo()
		if !usedNames[candidate] {
			return candidate
		}
	}

	baseName := pickRandomCombo()
	for i := 2; i < maxSuffixAttempts; i++ {
		candidate := baseName + "#" + strconv.Itoa(i)
		// v2-R-83: use rune count instead of byte length. The adjective/noun pools
		// contain Chinese runes (3 bytes each), so a 5-rune base + "#2" = 7 runes
		// but 17 bytes — byte length would wrongly skip valid candidates.
		if utf8.RuneCountInString(candidate) <= maxNicknameLength && !usedNames[candidate] {
			return candidate
		}
	}

	for i := 0; i < maxSuffixAttempts; i++ {
		candidate := "Player" + strconv.Itoa(randomIndex(10000))
		if !usedNames[candidate] {
			return candidate
		}
	}
	return "Player" + strconv.Itoa(randomIndex(10000))
}
