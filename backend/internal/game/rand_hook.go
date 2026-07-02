package game

import (
	"crypto/rand"
	"io"
	"math/big"
)

// randIntFn is injectable for unit tests (e.g. simulate crypto/rand failures).
var randIntFn = func(r io.Reader, max *big.Int) (*big.Int, error) {
	return rand.Int(r, max)
}

// SetRandIntHook overrides random int generation in tests and returns a restore func.
func SetRandIntHook(fn func(io.Reader, *big.Int) (*big.Int, error)) (restore func()) {
	prev := randIntFn
	randIntFn = fn
	return func() { randIntFn = prev }
}
