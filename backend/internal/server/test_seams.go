// Package server — test seams.
//
// This file consolidates all package-level test hook variables that exist
// solely for tests to override behavior (e.g. inject mock stores, simulate
// shutdown signals). Production code references these vars; tests swap
// them via direct assignment with t.Cleanup restore. Keeping them in one
// file improves discoverability and makes the test-seam surface explicit.
package server

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/telemetry"
)

// Test seam: filepathAbsFn resolves absolute paths; tests may replace it
// to simulate errors.
var filepathAbsFn = filepath.Abs

// Test seam: newPostgresStoreFn is replaceable in unit tests to inject
// pgxmock-backed stores.
var newPostgresStoreFn = store.NewPostgresStore

// Test seam: newRedisStoreFn is replaceable in unit tests.
var newRedisStoreFn = store.NewRedisStore

// Test seam: shutdownSignals returns the OS signal channel used for
// graceful shutdown. Tests may replace this to inject signals without
// sending real SIGTERM.
var shutdownSignals = func() <-chan os.Signal {
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)
	return done
}

// Test seam: initTracerFn is replaceable in unit tests.
var initTracerFn = telemetry.InitTracer

// Test seam: serverShutdownFn is replaceable in unit tests (http.Server.Shutdown).
var serverShutdownFn = func(srv *http.Server, ctx context.Context) error {
	return srv.Shutdown(ctx)
}

// Test seam: exitFunc is replaceable in unit tests (Run calls os.Exit on failure).
var exitFunc = os.Exit
