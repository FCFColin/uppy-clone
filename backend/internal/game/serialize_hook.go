package game

import (
	"github.com/uppy-clone/backend/internal/domain"
)

// serializeStateFn is injectable for unit tests (e.g. simulate serialize failures).
var serializeStateFn = SerializeState

// SetSerializeStateHook overrides state serialization in tests and returns a restore func.
func SetSerializeStateHook(fn func(*domain.GameState) ([]byte, error)) (restore func()) {
	prev := serializeStateFn
	serializeStateFn = fn
	return func() { serializeStateFn = prev }
}

