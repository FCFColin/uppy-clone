//go:build integration

package integration

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/testutil"
)

func TestRateLimiter_BasicAllow(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	ctx := context.Background()

	allowed, err := redisStore.CheckRateLimit(ctx, "test:basic", 5, time.Minute)
	if err != nil {
		t.Fatalf("CheckRateLimit: %v", err)
	}
	if !allowed {
		t.Fatal("expected first request to be allowed")
	}
}

func TestRateLimiter_MultipleRequestsWithinLimit(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	ctx := context.Background()

	key := "test:within"
	for i := 0; i < 3; i++ {
		allowed, err := redisStore.CheckRateLimit(ctx, key, 5, time.Minute)
		if err != nil {
			t.Fatalf("CheckRateLimit attempt %d: %v", i, err)
		}
		if !allowed {
			t.Fatalf("attempt %d should be allowed (limit=5)", i)
		}
	}
}

func TestRateLimiter_ExceedsLimit(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	ctx := context.Background()

	key := "test:exceed"
	limit := int64(3)
	window := time.Minute

	for i := int64(0); i < limit; i++ {
		allowed, err := redisStore.CheckRateLimit(ctx, key, limit, window)
		if err != nil {
			t.Fatalf("CheckRateLimit attempt %d: %v", i, err)
		}
		if !allowed {
			t.Fatalf("attempt %d should be allowed (within limit)", i)
		}
	}

	allowed, err := redisStore.CheckRateLimit(ctx, key, limit, window)
	if err != nil {
		t.Fatalf("CheckRateLimit over limit: %v", err)
	}
	if allowed {
		t.Fatal("expected request over limit to be denied")
	}
}

func TestRateLimiter_IndependentKeys(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	ctx := context.Background()

	allowed1, err := redisStore.CheckRateLimit(ctx, "test:key1", 1, time.Minute)
	if err != nil {
		t.Fatalf("CheckRateLimit key1: %v", err)
	}
	if !allowed1 {
		t.Fatal("expected key1 first request to be allowed")
	}

	allowed2, err := redisStore.CheckRateLimit(ctx, "test:key2", 1, time.Minute)
	if err != nil {
		t.Fatalf("CheckRateLimit key2: %v", err)
	}
	if !allowed2 {
		t.Fatal("expected key2 first request to be allowed (independent keys)")
	}

	over1, err := redisStore.CheckRateLimit(ctx, "test:key1", 1, time.Minute)
	if err != nil {
		t.Fatalf("CheckRateLimit key1 second: %v", err)
	}
	if over1 {
		t.Fatal("expected key1 second request to be denied")
	}
}

func TestRateLimiter_ConcurrentRequests(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	ctx := context.Background()

	key := "test:concurrent"
	limit := int64(5) // limit < goroutine count so some requests are denied
	window := time.Minute
	goroutines := 10

	var wg sync.WaitGroup
	var allowedCount int64
	var deniedCount int64
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed, err := redisStore.CheckRateLimit(ctx, key, limit, window)
			if err != nil {
				t.Errorf("concurrent CheckRateLimit failed: %v", err)
				return
			}
			if allowed {
				atomic.AddInt64(&allowedCount, 1)
			} else {
				atomic.AddInt64(&deniedCount, 1)
			}
		}()
	}
	wg.Wait()

	// With limit=5 and 10 concurrent requests, exactly `limit` should be allowed
	// and the rest denied (miniredis processes INCR atomically).
	if allowedCount != limit {
		t.Errorf("allowed = %d, want %d (limit)", allowedCount, limit)
	}
	wantDenied := int64(goroutines) - limit
	if deniedCount != wantDenied {
		t.Errorf("denied = %d, want %d", deniedCount, wantDenied)
	}
}

func TestRateLimiter_EmptyKey(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	ctx := context.Background()

	allowed, err := redisStore.CheckRateLimit(ctx, "", 5, time.Minute)
	if err != nil {
		t.Fatalf("CheckRateLimit empty key: %v", err)
	}
	if !allowed {
		t.Fatal("expected empty key request to be handled")
	}
}

func TestRateLimiter_ZeroLimit(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	ctx := context.Background()

	allowed, err := redisStore.CheckRateLimit(ctx, "test:zero", 0, time.Minute)
	if err != nil {
		t.Fatalf("CheckRateLimit zero limit: %v", err)
	}
	if allowed {
		t.Fatal("expected zero limit to deny immediately")
	}
}

func TestRateLimiter_DifferentWindows(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	ctx := context.Background()

	key := "test:windows"

	allowed, err := redisStore.CheckRateLimit(ctx, key, 5, 30*time.Second)
	if err != nil {
		t.Fatalf("CheckRateLimit 30s window: %v", err)
	}
	if !allowed {
		t.Fatal("expected first request to be allowed")
	}

	allowed, err = redisStore.CheckRateLimit(ctx, key, 5, 30*time.Second)
	if err != nil {
		t.Fatalf("CheckRateLimit second: %v", err)
	}
	if !allowed {
		t.Fatal("expected second request to be allowed (limit=5)")
	}
}
