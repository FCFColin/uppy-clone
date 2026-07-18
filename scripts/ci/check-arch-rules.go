// check-arch-rules verifies Go import dependency layering (logical architecture).
// It enforces which internal packages may import which others (e.g. handler must
// not import store). This is the "logical architecture" check.
//
// Responsibility boundary: this script checks IMPORT dependencies only.
// Physical file/directory layout is checked by check-repo-layout.go.
// The two scripts have no overlapping check logic.
package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

type archRule struct {
	pkgDir        string   // which package to check
	forbiddenPkgs []string // forbidden import prefixes
}

var rules = []archRule{
	{pkgDir: "internal/handler", forbiddenPkgs: []string{"internal/store", "internal/auth"}},
	{pkgDir: "internal/game", forbiddenPkgs: []string{"internal/store", "internal/auth", "internal/handler"}},
	{pkgDir: "internal/auth", forbiddenPkgs: []string{"internal/store"}},
	{pkgDir: "internal/middleware", forbiddenPkgs: []string{"internal/store", "internal/auth"}},
	{pkgDir: "internal/domain", forbiddenPkgs: []string{"internal/"}},
}

func main() {
	hasErrors := false
	basedir := "backend"
	if len(os.Args) > 1 {
		basedir = os.Args[1]
	}

	for _, rule := range rules {
		pkgPath := filepath.Join(basedir, rule.pkgDir)
		entries, err := os.ReadDir(pkgPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN: cannot read %s: %v\n", pkgPath, err)
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, filepath.Join(pkgPath, entry.Name()), nil, parser.ImportsOnly)
			if err != nil {
				continue
			}
			for _, imp := range f.Imports {
				impPath := strings.Trim(imp.Path.Value, `"`)
				for _, forbidden := range rule.forbiddenPkgs {
					if impPath == forbidden || strings.HasPrefix(impPath, forbidden+"/") {
						pos := fset.Position(imp.Pos())
						fmt.Fprintf(os.Stderr, "ERROR: %s imports forbidden package %s (%s)\n",
							pos, forbidden, impPath)
						hasErrors = true
					}
				}
			}
		}
	}

	if hasErrors {
		fmt.Println("❌ Architecture rule violations found")
		os.Exit(1)
	}
	fmt.Println("✅ All architecture rules pass")
}
