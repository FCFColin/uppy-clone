package nicknames

import (
	"crypto/rand"
	"io"
	"math/big"
)

// randIntFn is injectable for unit tests (e.g. simulate crypto/rand failures).
var randIntFn = rand.Int

// SetRandIntHook overrides random int generation in tests and returns a restore func.
func SetRandIntHook(fn func(io.Reader, *big.Int) (*big.Int, error)) (restore func()) {
	prev := randIntFn
	randIntFn = fn
	return func() { randIntFn = prev }
}
