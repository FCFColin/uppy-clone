package game

import "math/rand/v2"

type RNGSource interface {
	Float64() float64
	IntN(n int) int
}

type seededRNG struct {
	rng *rand.Rand
}

func (s *seededRNG) Float64() float64 {
	return s.rng.Float64()
}

func (s *seededRNG) IntN(n int) int {
	return s.rng.IntN(n)
}

func newSeededRNG(seed int64) *seededRNG {
	return &seededRNG{rng: rand.New(rand.NewPCG(uint64(seed), uint64(seed^0xDEADBEEF)))}
}
