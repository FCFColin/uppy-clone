package domain

import (
	"strings"
	"testing"
)

// fakeRNGSource is a deterministic rngSource for testing room code generation.
type fakeRNGSource struct {
	values []int
	idx    int
}

func (f *fakeRNGSource) IntN(n int) int {
	if f.idx >= len(f.values) {
		f.idx = 0
	}
	v := f.values[f.idx%len(f.values)]
	f.idx++
	if v >= n {
		return n - 1
	}
	return v
}

func TestGenerateRoomCode_Length(t *testing.T) {
	t.Parallel()
	rng := &fakeRNGSource{values: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}}
	code := GenerateRoomCode(rng)
	if len(code) != 5 {
		t.Fatalf("code length = %d, want 5", len(code))
	}
}

func TestGenerateRoomCode_UsesAlphabet(t *testing.T) {
	t.Parallel()
	rng := &fakeRNGSource{values: []int{0, 1, 2, 3, 4}}
	code := GenerateRoomCode(rng)
	// roomAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // pragma: allowlist secret
	want := "ABCDE"
	if code != want {
		t.Fatalf("code = %q, want %q", code, want)
	}
}

func TestGenerateRoomCode_AllCharsFromAlphabet(t *testing.T) {
	t.Parallel()
	rng := NewSeededRNG(1)
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // pragma: allowlist secret
	for i := 0; i < 100; i++ {
		code := GenerateRoomCode(rng)
		for _, c := range code {
			if !strings.ContainsRune(alphabet, c) {
				t.Fatalf("code %q contains invalid char %c", code, c)
			}
		}
	}
}

func TestNewSeededRNG_Deterministic(t *testing.T) {
	t.Parallel()
	r1 := NewSeededRNG(42)
	r2 := NewSeededRNG(42)
	for i := 0; i < 10; i++ {
		if r1.IntN(1000) != r2.IntN(1000) {
			t.Fatalf("seeded RNG not deterministic at iteration %d", i)
		}
	}
}

func TestNewSeededRNG_DifferentSeedsDiffer(t *testing.T) {
	t.Parallel()
	r1 := NewSeededRNG(1)
	r2 := NewSeededRNG(2)
	differ := false
	for i := 0; i < 10; i++ {
		if r1.IntN(1000) != r2.IntN(1000) {
			differ = true
			break
		}
	}
	if !differ {
		t.Fatal("different seeds should produce different sequences")
	}
}

func TestRoomCodeGenerator_GenerateRoomCode(t *testing.T) {
	t.Parallel()
	g := NewRoomCodeGenerator(12345)
	code := g.GenerateRoomCode()
	if len(code) != 5 {
		t.Fatalf("code length = %d, want 5", len(code))
	}
	if _, err := NewRoomCode(code); err != nil {
		t.Fatalf("generated code %q invalid: %v", code, err)
	}
}

func TestRoomCodeGenerator_DeterministicWithSameSeed(t *testing.T) {
	t.Parallel()
	g1 := NewRoomCodeGenerator(99)
	g2 := NewRoomCodeGenerator(99)
	if g1.GenerateRoomCode() != g2.GenerateRoomCode() {
		t.Fatal("same seed should produce same code")
	}
}

func TestRoomCodeGenerator_SetGenerateRoomCodeHook(t *testing.T) {
	t.Parallel()
	g := NewRoomCodeGenerator(1)

	restore := g.SetGenerateRoomCodeHook(func() string { return "HOOK1" })
	if g.GenerateRoomCode() != "HOOK1" {
		t.Fatal("hook should override code generation")
	}

	restore()
	// After restore, the hook is no longer active. The underlying RNG advances
	// on each call, so we can't compare to a pre-hook output — but we CAN
	// assert that the output no longer equals the hook value and looks like
	// a valid 5-char code from the generator's alphabet.
	got := g.GenerateRoomCode()
	if got == "HOOK1" {
		t.Fatal("restore should revert to original generator (output still equals hook value)")
	}
	if len(got) != 5 {
		t.Fatalf("after restore, code length = %d, want 5", len(got))
	}
	if _, err := NewRoomCode(got); err != nil {
		t.Fatalf("after restore, generated code %q invalid: %v", got, err)
	}
}

func TestRoomCodeGenerator_HookReplaceExisting(t *testing.T) {
	t.Parallel()
	g := NewRoomCodeGenerator(1)
	g.SetGenerateRoomCodeHook(func() string { return "FIRST" })
	g.SetGenerateRoomCodeHook(func() string { return "SECOND" })
	if got := g.GenerateRoomCode(); got != "SECOND" {
		t.Fatalf("got %q, want SECOND", got)
	}
}

// UUID tests live in uuid_test.go (TestUUID_Format, TestUUID_Uniqueness,
// TestUUID_ConcurrentUniqueness, TestUUID_NotAllZeros,
// TestUUID_DistributionSanity) — do not duplicate them here.
