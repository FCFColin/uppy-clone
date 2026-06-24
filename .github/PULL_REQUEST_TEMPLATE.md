## Summary

<!-- Brief description of what this PR changes -->

## Motivation

<!-- Why is this change needed? What problem does it solve? -->

## Changes

<!-- List the key changes made -->

-

## Test Plan

<!-- How did you verify these changes work? -->

- [ ] `go test -race ./...` passes
- [ ] `golangci-lint run` passes
- [ ] `govulncheck ./...` passes (no new high-severity CVEs)
- [ ] Manual testing performed (describe below)

## Checklist

- [ ] Code follows project style guidelines (`golangci-lint`, `gofmt`)
- [ ] Self-review completed
- [ ] Comments added for complex logic
- [ ] Documentation updated (`docs/`, `CHANGELOG.md`, `docs/openapi.yaml` if applicable)
- [ ] No new warnings from `go vet` or `golangci-lint`
- [ ] Tests added/updated for new functionality
- [ ] All existing tests pass with `go test -race ./...`
- [ ] Database migrations have both Up and Down scripts (if applicable)
- [ ] No secrets/credentials committed
- [ ] CHANGELOG.md updated (if user-facing change)

## Type of Change

- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Documentation update
- [ ] Refactoring (no functional changes)
- [ ] Performance improvement
- [ ] Security hardening
