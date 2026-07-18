package server

import (
	"context"
	"net/http"
	"path/filepath"

	"github.com/uppy-clone/backend/internal/bootstrap"
)

// ServerDeps holds injectable dependencies for the server lifecycle.
// Production code uses DefaultServerDeps(); tests construct custom instances
// to inject mocks without mutating package-level globals.
//
// Shared fields (InitTracer, NewPostgresStore, ShutdownSignals, Exit) live
// in the embedded bootstrap.Deps (spec remediate-structural-debt C3).
// Server-specific fields (ServerShutdown, FilepathAbs) are declared below.
type ServerDeps struct {
	bootstrap.Deps

	// ServerShutdown gracefully shuts down the HTTP server.
	ServerShutdown func(srv *http.Server, ctx context.Context) error

	// FilepathAbs resolves absolute paths (used for static-file path-traversal checks).
	FilepathAbs func(string) (string, error)
}

// DefaultServerDeps returns production-ready dependencies.
func DefaultServerDeps() ServerDeps {
	return ServerDeps{
		Deps:           bootstrap.DefaultDeps(),
		ServerShutdown: bootstrap.HTTPShutdown,
		FilepathAbs:    filepath.Abs,
	}
}
