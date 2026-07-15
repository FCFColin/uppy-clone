package idgen

import (
	"testing"

	"pgregory.net/rapid"
)

// TestUUID_PropertyMatchesV4Format: Every generated UUID matches the canonical
// v4 format (8-4-4-4-12 hex with version=4, variant=8/9/a/b).
func TestUUID_PropertyMatchesV4Format(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		id := UUID()
		if !uuidV4Pattern.MatchString(id) {
			t.Fatalf("UUID %q does not match v4 format", id)
		}
	})
}

// TestUUID_PropertyLengthIs36: Every generated UUID is exactly 36 characters.
func TestUUID_PropertyLengthIs36(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		id := UUID()
		if len(id) != 36 {
			t.Fatalf("UUID length = %d, want 36 (id=%q)", len(id), id)
		}
	})
}

// TestUUID_PropertyVersionNibbleIs4: The version nibble (char at index 14) is always '4'.
func TestUUID_PropertyVersionNibbleIs4(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		id := UUID()
		if id[14] != '4' {
			t.Fatalf("version char = %q, want '4' (id=%q)", string(id[14]), id)
		}
	})
}

// TestUUID_PropertyVariantNibbleValid: The variant nibble (char at index 19) is always
// one of 8/9/a/b (top two bits of byte 8 are 10).
func TestUUID_PropertyVariantNibbleValid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		id := UUID()
		switch id[19] {
		case '8', '9', 'a', 'b':
		default:
			t.Fatalf("variant char = %q, want one of 8/9/a/b (id=%q)", string(id[19]), id)
		}
	})
}

// TestUUID_PropertyNotAllZeros: The UUID is never the all-zeros-with-version-bits sentinel.
func TestUUID_PropertyNotAllZeros(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		id := UUID()
		if id == "00000000-0000-4000-8000-000000000000" {
			t.Fatal("UUID is the all-zeros sentinel")
		}
	})
}
