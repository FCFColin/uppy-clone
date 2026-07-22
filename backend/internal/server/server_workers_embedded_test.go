// Package server — EnableEmbeddedWorkers env parsing test.
//
// Verifies the env parser maps ENABLE_EMBEDDED_WORKERS to Env.EnableEmbeddedWorkers
// correctly. The flag gates in-process GDPR cleanup worker startup.
package server

import (
	"testing"

	appConfig "github.com/uppy-clone/backend/internal/config"
)

// TestEnv_EnableEmbeddedWorkers_Parsing is a companion test that verifies
// the env parser maps ENABLE_EMBEDDED_WORKERS to Env.EnableEmbeddedWorkers
// correctly. Keeps the flag's parsing contract pinned.
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
