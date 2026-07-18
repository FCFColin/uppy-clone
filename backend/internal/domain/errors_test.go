package domain

import (
	"errors"
	"testing"
)

func TestErrors_AreSentinelErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
	}{
		{"ErrDuplicateUser", ErrDuplicateUser},
		{"ErrNotFound", ErrNotFound},
		{"ErrValidation", ErrValidation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Fatalf("%s is nil", tt.name)
			}
			if !errors.Is(tt.err, tt.err) {
				t.Fatalf("%s should be identity-equal under errors.Is", tt.name)
			}
		})
	}
}

func TestErrors_DistinctMessages(t *testing.T) {
	t.Parallel()
	seen := make(map[string]bool)
	for _, err := range []error{ErrDuplicateUser, ErrNotFound, ErrValidation} {
		msg := err.Error()
		if msg == "" {
			t.Fatalf("error %v has empty message", err)
		}
		if seen[msg] {
			t.Fatalf("duplicate error message: %q", msg)
		}
		seen[msg] = true
	}
}

func TestErrors_Wrappable(t *testing.T) {
	t.Parallel()
	wrapped := errors.Join(ErrNotFound, ErrValidation)
	if !errors.Is(wrapped, ErrNotFound) {
		t.Fatal("wrapped should be ErrNotFound")
	}
	if !errors.Is(wrapped, ErrValidation) {
		t.Fatal("wrapped should be ErrValidation")
	}
}
