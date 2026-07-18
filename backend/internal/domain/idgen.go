package domain

import (
	crand "crypto/rand"
	"fmt"
	"math/rand/v2"
	"sync"

	"github.com/uppy-clone/backend/internal/config"
)

// rngSource is the minimal RNG interface needed for room code generation.
// Go's structural typing means any type with IntN(n int) int satisfies this.
type rngSource interface {
	IntN(n int) int
}

// seededRNG wraps math/rand/v2 for reproducible sequences (game RNG, not crypto).
type seededRNG struct {
	rng *rand.Rand
}

func (s *seededRNG) IntN(n int) int { return s.rng.IntN(n) }

// NewSeededRNG creates a deterministic RNG from a seed.
func NewSeededRNG(seed int64) *seededRNG {
	return &seededRNG{rng: rand.New(rand.NewPCG(uint64(seed), uint64(seed^0xDEADBEEF)))} //nolint:gosec // G404: game RNG, not crypto
}

const roomAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// GenerateRoomCode generates a config.RoomCodeLen character room code.
func GenerateRoomCode(rng rngSource) string {
	code := make([]byte, config.RoomCodeLen)
	for i := range code {
		code[i] = roomAlphabet[rng.IntN(len(roomAlphabet))]
	}
	return string(code)
}

// RoomCodeGenerator generates unique room codes. The RNG state and hook-override
// logic are isolated from the room registry.
type RoomCodeGenerator struct {
	rng   rngSource
	mu    sync.Mutex
	genFn func() string
}

// NewRoomCodeGenerator creates a RoomCodeGenerator seeded with the given value.
func NewRoomCodeGenerator(seed int64) *RoomCodeGenerator {
	g := &RoomCodeGenerator{
		rng: NewSeededRNG(seed),
	}
	g.genFn = func() string {
		return GenerateRoomCode(g.rng)
	}
	return g
}

// SetGenerateRoomCodeHook overrides room code generation and returns a
// restore function to revert to the original behavior.
func (g *RoomCodeGenerator) SetGenerateRoomCodeHook(fn func() string) (restore func()) {
	g.mu.Lock()
	orig := g.genFn
	g.genFn = fn
	g.mu.Unlock()
	return func() {
		g.mu.Lock()
		g.genFn = orig
		g.mu.Unlock()
	}
}

// GenerateRoomCode returns a room code, using the hook if set, otherwise
// the default RNG-based generator.
func (g *RoomCodeGenerator) GenerateRoomCode() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.genFn()
}

// UUID generates a v4 UUID string using crypto/rand.
func UUID() string {
	b := make([]byte, 16)
	if _, err := crand.Read(b); err != nil {
		panic("crypto/rand.Read failed: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
