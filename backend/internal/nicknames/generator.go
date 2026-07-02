package nicknames

import (
	"crypto/rand"
	"math/big"
	"strconv"
)

const maxNicknameLength = 12

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
	for i := 2; i < 100; i++ {
		candidate := baseName + "#" + strconv.Itoa(i)
		if len(candidate) <= maxNicknameLength && !usedNames[candidate] {
			return candidate
		}
	}

	return "Player" + strconv.Itoa(randomIndex(10000))
}
