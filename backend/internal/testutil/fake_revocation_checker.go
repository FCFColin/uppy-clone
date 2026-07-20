package testutil

import "context"

// FakeRevocationChecker is a test double for auth.JWTRevocationChecker.
// It is shared across the auth and middleware packages' tests to avoid
// duplicating the same trivial in-memory revocation checker.
type FakeRevocationChecker struct {
	Revoked map[string]bool
	Err     error
}

// NewFakeRevocationChecker creates a FakeRevocationChecker with an empty
// revoked set.
func NewFakeRevocationChecker() *FakeRevocationChecker {
	return &FakeRevocationChecker{Revoked: make(map[string]bool)}
}

// IsJWTRevoked returns the configured error (if any) or whether the jti has
// been marked as revoked.
func (f *FakeRevocationChecker) IsJWTRevoked(_ context.Context, jti string) (bool, error) {
	if f.Err != nil {
		return false, f.Err
	}
	return f.Revoked[jti], nil
}
