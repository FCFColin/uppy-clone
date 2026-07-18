// Package bootstrap holds shared wiring used by both the server process
// (internal/server) and the standalone game-worker process
// (internal/worker runner). Extracting these types and helpers here
// eliminates byte-level duplication between server_init.go and
// worker/runner.go (spec remediate-structural-debt C3).
package bootstrap

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	appConfig "github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/telemetry"
)

// Deps holds injectable dependencies shared by the server and worker
// processes. Production code uses DefaultDeps(); tests construct custom
// instances by calling DefaultDeps() then reassigning individual fields
// to inject mocks without mutating package-level globals.
//
// Process-specific deps (e.g. server.ServerDeps.ServerShutdown,
// worker.WorkerDeps.NewRedisCluster) embed this struct and add their own
// fields on top — see server.ServerDeps and worker.WorkerDeps.
type Deps struct {
	// InitTracer initialises OpenTelemetry tracing.
	InitTracer func(ctx context.Context, serviceName, serviceVersion string, cfg telemetry.TracerConfig) (func(context.Context) error, error)

	// NewPostgresStore creates a PostgresStore. Tests inject pgxmock-backed pools.
	NewPostgresStore func(dsn string, timeouts appConfig.TimeoutConfig, deps ...store.Deps) (*store.PostgresStore, error)

	// ShutdownSignals returns a channel that receives OS shutdown signals.
	ShutdownSignals func() <-chan os.Signal

	// Exit terminates the process. Tests inject a no-op to prevent os.Exit.
	Exit func(code int)
}

// DefaultDeps returns production-ready shared dependencies.
func DefaultDeps() Deps {
	return Deps{
		InitTracer:       telemetry.InitTracer,
		NewPostgresStore: store.NewPostgresStore,
		ShutdownSignals:  DefaultShutdownSignals,
		Exit:             os.Exit,
	}
}

// DefaultShutdownSignals returns a channel that receives SIGINT/SIGTERM.
// Shared by server and worker (previously duplicated as
// server.defaultShutdownSignals and worker.defaultWorkerShutdownSignals).
func DefaultShutdownSignals() <-chan os.Signal {
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)
	return done
}

// HTTPShutdown gracefully shuts down an HTTP server. Used as the default
// ServerShutdown (server) and HTTPShutdown (worker) impl. Previously
// duplicated as server.defaultServerShutdown and worker.defaultWorkerHTTPShutdown.
func HTTPShutdown(srv *http.Server, ctx context.Context) error {
	return srv.Shutdown(ctx)
}
