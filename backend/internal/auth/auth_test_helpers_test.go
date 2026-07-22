package auth

import (
	"testing"

	"github.com/uppy-clone/backend/internal/testsecrets"
	"github.com/uppy-clone/backend/internal/testutil"
)

// setupRefreshEnv builds a JWT manager and a refresh-token manager backed by a
// fresh miniredis instance. The miniredis is registered for t.Cleanup.
//
// E5: consolidates the 7-line `miniredis.Run + t.Fatalf + t.Cleanup +
// NewJWTManager + NewRefreshTokenManager(redis.NewClient(...))` block that
// appeared 15+ times across quickplay_flow_test.go, magiclink_verify_test.go,
// and auth_misc_test.go.
func setupRefreshEnv(t *testing.T) (*JWTManager, *RefreshTokenManager) {
	t.Helper()
	_, rdb := testutil.NewTestMiniredis(t)
	return NewJWTManager(testsecrets.TestJWTPrivateKeyPEM), NewRefreshTokenManager(rdb)
}

// setupJWTManager returns a JWT manager using the shared test private key.
// E5: wraps `NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)` (10+ sites).
func setupJWTManager() *JWTManager {
	return NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
}
