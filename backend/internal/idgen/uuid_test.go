package idgen

import (
	"regexp"
	"sync"
	"testing"
)

// uuidV4Pattern matches RFC 4122 v4 UUIDs:
//   - 8 hex - 4 hex - 4 hex (version 4) - 4 hex (variant 8/9/a/b) - 12 hex
var uuidV4Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// TestUUID_Format verifies that UUID() output conforms to the RFC 4122 v4 format.
func TestUUID_Format(t *testing.T) {
	for i := 0; i < 1000; i++ {
		id := UUID()
		if !uuidV4Pattern.MatchString(id) {
			t.Fatalf("UUID %q does not match v4 format (iteration %d)", id, i)
		}
	}
}

// TestUUID_Length verifies the UUID is always 36 characters (32 hex + 4 hyphens).
func TestUUID_Length(t *testing.T) {
	for i := 0; i < 100; i++ {
		id := UUID()
		if len(id) != 36 {
			t.Fatalf("UUID length = %d, want 36 (id=%q)", len(id), id)
		}
	}
}

// TestUUID_Version verifies the version nibble (char at index 14) is always '4'.
func TestUUID_Version(t *testing.T) {
	for i := 0; i < 1000; i++ {
		id := UUID()
		if id[14] != '4' {
			t.Fatalf("UUID version nibble = %c, want '4' (id=%q)", id[14], id)
		}
	}
}

// TestUUID_Variant verifies the variant nibble (char at index 19) is one of 8/9/a/b.
func TestUUID_Variant(t *testing.T) {
	validVariant := map[byte]bool{'8': true, '9': true, 'a': true, 'b': true}
	for i := 0; i < 1000; i++ {
		id := UUID()
		if !validVariant[id[19]] {
			t.Fatalf("UUID variant nibble = %c, want one of 8/9/a/b (id=%q)", id[19], id)
		}
	}
}

// TestUUID_Uniqueness verifies that generating many UUIDs produces no duplicates.
// This is adversarial: a buggy implementation using a fixed seed would fail.
func TestUUID_Uniqueness(t *testing.T) {
	const n = 100_000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := UUID()
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate UUID %q after %d generations", id, i)
		}
		seen[id] = struct{}{}
	}
}

// TestUUID_ConcurrentUniqueness verifies uniqueness under concurrent generation.
// Runs with -race to detect data races in the random source.
func TestUUID_ConcurrentUniqueness(t *testing.T) {
	const goroutines = 50
	const perGoroutine = 2000

	var mu sync.Mutex
	seen := make(map[string]struct{}, goroutines*perGoroutine)
	var wg sync.WaitGroup

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			localIDs := make([]string, 0, perGoroutine)
			for i := 0; i < perGoroutine; i++ {
				localIDs = append(localIDs, UUID())
			}
			mu.Lock()
			for _, id := range localIDs {
				if _, exists := seen[id]; exists {
					t.Errorf("duplicate UUID %q in concurrent generation", id)
				}
				seen[id] = struct{}{}
			}
			mu.Unlock()
		}()
	}
	wg.Wait()

	expected := goroutines * perGoroutine
	if len(seen) != expected {
		t.Fatalf("unique count = %d, want %d", len(seen), expected)
	}
}

// TestUUID_HyphenPositions verifies hyphens are at the expected positions (8, 13, 18, 23).
func TestUUID_HyphenPositions(t *testing.T) {
	for i := 0; i < 100; i++ {
		id := UUID()
		hyphenPositions := []int{8, 13, 18, 23}
		for _, pos := range hyphenPositions {
			if id[pos] != '-' {
				t.Fatalf("expected hyphen at position %d, got %c (id=%q)", pos, id[pos], id)
			}
		}
	}
}

// TestUUID_NotAllZeros verifies the UUID is not the nil UUID (all zeros).
// This is adversarial: if rand.Read fails silently and returns all-zero bytes,
// the version/variant bits would still be set, but the rest would be zero.
func TestUUID_NotAllZeros(t *testing.T) {
	for i := 0; i < 100; i++ {
		id := UUID()
		// Strip hyphens and check it's not "40000000000000000000000000000" pattern
		// (version 4 + variant 8 but everything else zero would indicate rand failure)
		if id == "00000000-0000-4000-8000-000000000000" {
			t.Fatalf("UUID appears to be all-zeros with only version/variant bits set: %q", id)
		}
	}
}

// TestUUID_DistributionSanity verifies that the first byte has reasonable distribution.
// This is adversarial: a broken random source would cluster all values on one byte.
func TestUUID_DistributionSanity(t *testing.T) {
	const n = 10_000
	firstByteCounts := make(map[byte]int)
	for i := 0; i < n; i++ {
		id := UUID()
		// Parse the first two hex chars as a byte value.
		var b byte
		for j := 0; j < 2; j++ {
			c := id[j]
			b <<= 4
			switch {
			case c >= '0' && c <= '9':
				b |= c - '0'
			case c >= 'a' && c <= 'f':
				b |= c - 'a' + 10
			}
		}
		firstByteCounts[b]++
	}

	// With 10000 samples over 256 possible byte values, each value should
	// appear roughly 39 times (10000/256). We check that at least 200 distinct
	// byte values appeared, which rules out a degenerate random source.
	if len(firstByteCounts) < 200 {
		t.Fatalf("first byte distribution too narrow: only %d distinct values out of 256", len(firstByteCounts))
	}
}

// BenchmarkUUID benchmarks UUID generation performance.
func BenchmarkUUID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = UUID()
	}
}

// BenchmarkUUID_Parallel benchmarks parallel UUID generation.
func BenchmarkUUID_Parallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = UUID()
		}
	})
}
