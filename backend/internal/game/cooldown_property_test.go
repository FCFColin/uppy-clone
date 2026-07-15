package game

import (
	"testing"

	"github.com/uppy-clone/backend/internal/protocol"
	"pgregory.net/rapid"
)

// TestCalculateCooldown_PropertyAlwaysWithinBounds: For any player count,
// the cooldown is always in [CooldownBaseMs, CooldownMaxMs].
func TestCalculateCooldown_PropertyAlwaysWithinBounds(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		playerCount := rapid.Int().Draw(t, "playerCount")
		cd := CalculateCooldown(playerCount)
		if cd < protocol.CooldownBaseMs {
			t.Fatalf("cooldown %d < base %d for playerCount %d", cd, protocol.CooldownBaseMs, playerCount)
		}
		if cd > protocol.CooldownMaxMs {
			t.Fatalf("cooldown %d > max %d for playerCount %d", cd, protocol.CooldownMaxMs, playerCount)
		}
	})
}

// TestCalculateCooldown_PropertyMonotonicNonDecreasing: For player counts >= 1,
// the cooldown is monotonically non-decreasing (larger roster => longer cooldown).
func TestCalculateCooldown_PropertyMonotonicNonDecreasing(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n2 := rapid.IntRange(1, 10000).Draw(t, "n2")
		n1 := rapid.IntRange(1, n2).Draw(t, "n1")
		cd1 := CalculateCooldown(n1)
		cd2 := CalculateCooldown(n2)
		if cd1 > cd2 {
			t.Fatalf("cooldown not monotonic: cd(%d)=%d > cd(%d)=%d", n1, cd1, n2, cd2)
		}
	})
}

// TestCalculateCooldown_PropertySmallCountIsBase: For player count <= 1,
// the cooldown equals CooldownBaseMs (log2(max(1,N)) = log2(1) = 0).
func TestCalculateCooldown_PropertySmallCountIsBase(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		playerCount := rapid.IntRange(-1000, 1).Draw(t, "playerCount")
		cd := CalculateCooldown(playerCount)
		if cd != protocol.CooldownBaseMs {
			t.Fatalf("cooldown for playerCount %d = %d, want base %d", playerCount, cd, protocol.CooldownBaseMs)
		}
	})
}

// TestCalculateCooldown_PropertyLargeCountClampedToMax: For very large player counts,
// the cooldown is clamped to CooldownMaxMs.
func TestCalculateCooldown_PropertyLargeCountClampedToMax(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		playerCount := rapid.IntRange(1000, 1_000_000).Draw(t, "playerCount")
		cd := CalculateCooldown(playerCount)
		if cd != protocol.CooldownMaxMs {
			t.Fatalf("cooldown for playerCount %d = %d, want max %d", playerCount, cd, protocol.CooldownMaxMs)
		}
	})
}

// TestCalculateCooldown_PropertyDeterministic: CalculateCooldown is a pure function;
// the same input always yields the same output.
func TestCalculateCooldown_PropertyDeterministic(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		playerCount := rapid.Int().Draw(t, "playerCount")
		cd1 := CalculateCooldown(playerCount)
		cd2 := CalculateCooldown(playerCount)
		if cd1 != cd2 {
			t.Fatalf("non-deterministic: CalculateCooldown(%d) = %d then %d", playerCount, cd1, cd2)
		}
	})
}
