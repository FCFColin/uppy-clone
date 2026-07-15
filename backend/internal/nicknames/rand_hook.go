package nicknames

import (
	"crypto/rand"
	"io"
	"math/big"
)

// randIntFn is injectable for unit tests (e.g. simulate crypto/rand failures).
// misc-011: This var is NOT concurrent-safe — SetRandIntHook should only be called
// from test setup (t.Cleanup restores the original). In production, randIntFn is
// never modified, so the race-free read path is safe. If concurrent hook swapping
// is ever needed, wrap in sync.RWMutex.
var randIntFn = rand.Int

// SetRandIntHook overrides random int generation in tests and returns a restore func.
func SetRandIntHook(fn func(io.Reader, *big.Int) (*big.Int, error)) (restore func()) {
	prev := randIntFn
	randIntFn = fn
	return func() { randIntFn = prev }
}
