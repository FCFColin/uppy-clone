package auth

import "crypto/rand"

// randRead is injectable for unit tests (e.g. simulate crypto/rand failures).
var randRead = rand.Read

// SetRandReadHook overrides crypto/rand.Read in tests and returns a restore func.
func SetRandReadHook(fn func([]byte) (int, error)) (restore func()) {
	prev := randRead
	randRead = fn
	return func() { randRead = prev }
}
