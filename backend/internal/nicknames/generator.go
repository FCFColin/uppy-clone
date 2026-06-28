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
	r, err := rand.Int(rand.Reader, bigN)
	if err != nil {
		return 0
	}
	return int(r.Int64())
}

// GenerateRandom produces a random nickname from the word pools.
// If all attempts collide with usedNames, appends #N suffix or falls back to PlayerXXXX.
func GenerateRandom(usedNames map[string]bool) string {
	for i := 0; i < 10; i++ {
		adj := NicknameAdjectives[randomIndex(len(NicknameAdjectives))]
		cat := NicknameCategories[randomIndex(len(NicknameCategories))]
		noun := cat[randomIndex(len(cat))]
		candidate := adj + noun
		if !usedNames[candidate] {
			return candidate
		}
	}

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

	return "Player" + strconv.Itoa(randomIndex(10000))
}
