package requestctx

import (
	"context"
	"testing"
)

func TestWithTrustedProxy(t *testing.T) {
	t.Parallel()
	ctx := WithTrustedProxy(context.Background(), true)
	if !IsTrustedProxy(ctx) {
		t.Error("IsTrustedProxy should return true after WithTrustedProxy(ctx, true)")
	}
}

func TestWithTrustedProxy_False(t *testing.T) {
	t.Parallel()
	ctx := WithTrustedProxy(context.Background(), false)
	if IsTrustedProxy(ctx) {
		t.Error("IsTrustedProxy should return false after WithTrustedProxy(ctx, false)")
	}
}

func TestIsTrustedProxy_EmptyContext(t *testing.T) {
	t.Parallel()
	if IsTrustedProxy(context.Background()) {
		t.Error("IsTrustedProxy should return false on empty context")
	}
}

func TestIsTrustedProxy_WrongValueType(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), proxyKey{}, "yes")
	if IsTrustedProxy(ctx) {
		t.Error("IsTrustedProxy should return false for non-bool value")
	}
}

func TestIsTrustedProxy_Override(t *testing.T) {
	t.Parallel()
	ctx := WithTrustedProxy(context.Background(), true)
	ctx = WithTrustedProxy(ctx, false)
	if IsTrustedProxy(ctx) {
		t.Error("IsTrustedProxy should return false after override with false")
	}
}
