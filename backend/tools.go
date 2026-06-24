//go:build tools

package main

import (
	// Tool dependencies locked via go.mod
	_ "github.com/air-verse/air"
	_ "github.com/golang-migrate/migrate/v4"
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
)
