// check-repo-layout verifies the physical file/directory layout matches ADR-021
// (monorepo structure). It asserts legacy flat paths do not exist and required
// canonical paths do exist. This is the "physical layout" check.
//
// Responsibility boundary: this script checks FILE SYSTEM layout only.
// Go import dependency layering is checked by check-arch-rules.go.
// The two scripts have no overlapping check logic.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func repoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "backend", "cmd")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "frontend")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("repo root (backend/frontend) not found")
		}
		dir = parent
	}
}

func assertMissing(root, path string) bool {
	full := filepath.Join(root, path)
	if _, err := os.Stat(full); err == nil {
		fmt.Fprintf(os.Stderr, "legacy path must not exist: %s\n", full)
		return false
	}
	return true
}

func assertExists(root, path string) bool {
	full := filepath.Join(root, path)
	if _, err := os.Stat(full); err != nil {
		fmt.Fprintf(os.Stderr, "required path missing: %s\n", full)
		return false
	}
	return true
}

func main() {
	root := repoRoot()
	fail := false

	// Legacy flat docs (nested copies are canonical)
	legacyDocs := []string{
		"docs/runbook.md", "docs/slo.md", "docs/architecture.md", "docs/openapi.yaml",
		"docs/asyncapi.yaml", "docs/ws-protocol.md", "docs/coverage-policy.md",
		"docs/benchmarks-v2.md", "docs/environments.md", "docs/logging-policy.md",
		"docs/threat-model.md", "docs/multi-region-topology.md", "docs/cockroachdb-migration.md",
		"docs/db-query-analysis.md", "docs/capacity-planning.md", "docs/continuous-profiling.md",
		"docs/chaos-experiments.md",
	}
	for _, f := range legacyDocs {
		fail = !assertMissing(root, f) || fail
	}

	// Legacy backend server package in cmd/
	entries, err := os.ReadDir(filepath.Join(root, "backend", "cmd", "server"))
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") && e.Name() != "main.go" {
				fmt.Fprintf(os.Stderr, "legacy server file must not exist: backend/cmd/server/%s\n", e.Name())
				fail = true
			}
		}
	}

	// Legacy infra flat layout
	legacyInfra := []string{
		"infra/base", "infra/overlays", "infra/global",
		"infra/main.tf", "infra/variables.tf", "infra/outputs.tf", "infra/service.yaml",
	}
	for _, p := range legacyInfra {
		fail = !assertMissing(root, p) || fail
	}

	// Legacy scripts at repo scripts/ root
	legacyScripts := []string{
		"scripts/check-coverage.sh", "scripts/check-docker-digests.sh",
		"scripts/k6", "scripts/merge_go_tests.py", "scripts/merge-package-tests.py",
	}
	for _, f := range legacyScripts {
		fail = !assertMissing(root, f) || fail
	}

	// Legacy docker / frontend / rbac paths
	legacyMisc := []string{
		"docker/init-scripts",
		"frontend/play.css",
		"frontend/src/index_fetch.ts",
		"backend/internal/rbac/model.conf",
		"backend/internal/rbac/policy.csv",
	}
	for _, p := range legacyMisc {
		fail = !assertMissing(root, p) || fail
	}

	// Done -- no other checks needed.

	// Required canonical paths
	required := []string{
		"backend/internal/server",
		"infra/k8s/base", "infra/terraform",
		"deploy/local",
		"scripts/ci",
		"docker/postgres/init",
		"docs/operations/runbook.md",
		"docs/development/benchmarks-go-microbench.md",
	}
	for _, p := range required {
		fail = !assertExists(root, p) || fail
	}

	// cmd/server must contain only main.go
	serverEntries, err := os.ReadDir(filepath.Join(root, "backend", "cmd", "server"))
	if err == nil {
		var goFiles []string
		for _, e := range serverEntries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
				goFiles = append(goFiles, e.Name())
			}
		}
		if len(goFiles) != 1 || goFiles[0] != "main.go" {
			fmt.Fprintf(os.Stderr, "backend/cmd/server must contain only main.go (found: %v)\n", goFiles)
			fail = true
		}
	}

	if fail {
		os.Exit(1)
	}
	fmt.Println("repo layout OK (ADR-021)")
}