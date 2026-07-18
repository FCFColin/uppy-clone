// Package server — EnableEmbeddedWorkers flag control tests (spec
// slim-and-complete-architecture Task 3.10).
//
// These tests verify that startWorkers honors the ENABLE_EMBEDDED_WORKERS flag:
//
//   - ENABLE_EMBEDDED_WORKERS=false (opt-out): the server process
//     skips GameResult/Outbox/GDPR workers. A standalone game-worker Deployment
//     owns those consumers. We assert this by checking that the game.events
//     Redis Stream has NO "result-workers" consumer group after startWorkers
//     runs — GameResultWorker.Start calls XGroupCreateMkStream synchronously
//     on startup, so the absence of the group means the worker never started.
//
//   - ENABLE_EMBEDDED_WORKERS=true (production default): the server process
//     starts the workers in-process. We assert the "result-workers" group IS
//     created on the game.events stream.
//
// Email worker is separately gated by ResendAPIKey (tested elsewhere); both
// cases here leave ResendAPIKey empty so the email worker never starts, keeping
// the assertion focused on the GameResult/Outbox/GDPR tier.
package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	appConfig "github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/handler"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/testutil"
)

// assertGameResultGroupExists polls the game.events stream's consumer groups
// for up to timeout, returning true once "result-workers" appears. Used to
// detect that GameResultWorker.Start ran its XGroupCreateMkStream call.
func assertGameResultGroupExists(t *testing.T, client *redis.Client, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		groups, err := client.XInfoGroups(context.Background(), "game.events").Result()
		if err == nil {
			for _, g := range groups {
				if g.Name == "result-workers" {
					return true
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// assertGameResultGroupAbsent waits briefly then confirms the game.events
// stream has no "result-workers" group (and likely does not exist at all).
// We wait a short grace period to give any would-be worker goroutine time to
// reach XGroupCreateMkStream if it were going to run — absence after that
// window means startWorkers did NOT start the worker.
func assertGameResultGroupAbsent(t *testing.T, client *redis.Client, grace time.Duration) bool {
	t.Helper()
	time.Sleep(grace)
	groups, err := client.XInfoGroups(context.Background(), "game.events").Result()
	if err != nil {
		// Stream doesn't exist → group definitely absent. This is the expected
		// state when EnableEmbeddedWorkers=false and no other process has
		// written to the stream.
		return true
	}
	for _, g := range groups {
		if g.Name == "result-workers" {
			return false
		}
	}
	return true
}

// TestStartWorkers_EnableEmbeddedWorkersFalse_SkipsGameWorkers verifies the
// opt-out path: when ENABLE_EMBEDDED_WORKERS is false, the server
// process does NOT start GameResult/Outbox/GDPR workers. Those consumers are
// owned by the standalone game-worker process (worker.yaml).
//
// Assertion: after startWorkers runs, the game.events Redis Stream has no
// "result-workers" consumer group (the worker's XGroupCreateMkStream never
// fired). This is the load-bearing proof for spec Task 3.4 / 3.10.
func TestStartWorkers_EnableEmbeddedWorkersFalse_SkipsGameWorkers(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	mock := testutil.NewPgxMock(t)
	db := store.NewPostgresStoreWithPool(mock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup

	// Production default: embedded workers disabled, no Resend key.
	cfg := &handler.Config{
		EnableEmbeddedWorkers: false,
		// ResendAPIKey empty → email worker also skipped.
	}

	startWorkers(ctx, &wg, cfg, redisStore, db, appConfig.DefaultTimeoutConfig())

	// Give any would-be worker goroutine time to reach XGroupCreateMkStream.
	// GameResultWorker.Start fires it synchronously on entry, so 200ms is
	// plenty for the goroutine scheduler to run it if startWorkers had
	// launched it. Absence after this window = worker never started.
	if !assertGameResultGroupAbsent(t, redisStore.Client(), 200*time.Millisecond) {
		t.Fatal("EnableEmbeddedWorkers=false but game.events result-workers group was created — startWorkers must not start GameResult worker when flag is false")
	}

	cancel()
	wg.Wait()
}

// TestStartWorkers_EnableEmbeddedWorkersTrue_StartsGameResultWorker verifies
// the production default path: when ENABLE_EMBEDDED_WORKERS is true (or unset), the
// server process starts GameResult/Outbox/GDPR workers in-process.
//
// Assertion: the game.events Redis Stream has a "result-workers" consumer
// group shortly after startWorkers runs (GameResultWorker.Start calls
// XGroupCreateMkStream synchronously).
func TestStartWorkers_EnableEmbeddedWorkersTrue_StartsGameResultWorker(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	mock := testutil.NewPgxMock(t)
	db := store.NewPostgresStoreWithPool(mock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup

	cfg := &handler.Config{
		EnableEmbeddedWorkers: true,
		// ResendAPIKey empty → email worker skipped (we only care about the
		// GameResult/Outbox/GDPR tier here).
		// OutboxPollIntervalMs default 1000ms → outbox publisher won't tick
		// within our assertion window; GDPR cleanup interval is 24h → won't tick.
		// So neither will hit pgxmock with unexpected queries during the test.
	}

	startWorkers(ctx, &wg, cfg, redisStore, db, appConfig.DefaultTimeoutConfig())

	// GameResultWorker.Start calls XGroupCreateMkStream synchronously; the
	// goroutine scheduler should run it within 500ms. If the group never
	// appears, the flag gating is broken.
	if !assertGameResultGroupExists(t, redisStore.Client(), 500*time.Millisecond) {
		t.Fatal("EnableEmbeddedWorkers=true but game.events result-workers group was NOT created — startWorkers must start GameResult worker when flag is true")
	}

	cancel()
	wg.Wait()
}

// TestEnv_EnableEmbeddedWorkers_Parsing is a tiny companion test that verifies
// the env parser maps ENABLE_EMBEDDED_WORKERS to Env.EnableEmbeddedWorkers
// correctly. Keeps the flag's parsing contract pinned alongside its
// behavioral test.
func TestEnv_EnableEmbeddedWorkers_Parsing(t *testing.T) {
	t.Setenv("ENABLE_EMBEDDED_WORKERS", "false")
	env := appConfig.Load()
	if env.EnableEmbeddedWorkers {
		t.Error(`ENABLE_EMBEDDED_WORKERS="false" should parse to false`)
	}

	t.Setenv("ENABLE_EMBEDDED_WORKERS", "true")
	env = appConfig.Load()
	if !env.EnableEmbeddedWorkers {
		t.Error(`ENABLE_EMBEDDED_WORKERS="true" should parse to true`)
	}

	t.Setenv("ENABLE_EMBEDDED_WORKERS", "1")
	env = appConfig.Load()
	if !env.EnableEmbeddedWorkers {
		t.Error(`ENABLE_EMBEDDED_WORKERS="1" should parse to true`)
	}

	t.Setenv("ENABLE_EMBEDDED_WORKERS", "")
	env = appConfig.Load()
	if !env.EnableEmbeddedWorkers {
		t.Error(`ENABLE_EMBEDDED_WORKERS="" (unset) should parse to true (default)`)
	}
}
