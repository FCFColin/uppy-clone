# ADR-021 repo layout check (PowerShell). Mirrors scripts/ci/check-repo-layout.sh for Windows.
$ErrorActionPreference = 'Continue'
$Root = Resolve-Path (Join-Path $PSScriptRoot '../..')
Set-Location $Root
$script:fail = 0

function Assert-Missing($rel) {
    if (Test-Path $rel) {
        Write-Host "legacy path must not exist: $rel" -ForegroundColor Red
        $script:fail = 1
    }
}

function Assert-Exists($rel) {
    if (-not (Test-Path $rel)) {
        Write-Host "required path missing: $rel" -ForegroundColor Red
        $script:fail = 1
    }
}

$legacyDocs = @(
    'docs/runbook.md', 'docs/slo.md', 'docs/architecture.md', 'docs/openapi.yaml',
    'docs/asyncapi.yaml', 'docs/ws-protocol.md', 'docs/coverage-policy.md',
    'docs/benchmarks-v2.md', 'docs/environments.md', 'docs/logging-policy.md',
    'docs/threat-model.md', 'docs/multi-region-topology.md', 'docs/cockroachdb-migration.md',
    'docs/db-query-analysis.md', 'docs/capacity-planning.md', 'docs/continuous-profiling.md',
    'docs/chaos-experiments.md'
)
foreach ($f in $legacyDocs) { Assert-Missing $f }

Get-ChildItem 'backend/cmd/server/*.go' -ErrorAction SilentlyContinue | ForEach-Object {
    if ($_.Name -ne 'main.go') {
        Write-Host "legacy server file must not exist: $($_.FullName)" -ForegroundColor Red
        $script:fail = 1
    }
}

@('infra/base', 'infra/overlays', 'infra/global', 'infra/main.tf', 'infra/variables.tf', 'infra/outputs.tf', 'infra/service.yaml') | ForEach-Object { Assert-Missing $_ }
@('scripts/check-coverage.sh', 'scripts/check-docker-digests.sh', 'scripts/pin-digests.sh', 'scripts/k6', 'scripts/merge_go_tests.py', 'scripts/merge-package-tests.py') | ForEach-Object { Assert-Missing $_ }
@('docker/init-scripts', 'frontend/play.css', 'frontend/src/index_fetch.ts', 'backend/internal/rbac/model.conf', 'backend/internal/rbac/policy.csv', 'scripts/archive') | ForEach-Object { Assert-Missing $_ }

@(
    'backend/internal/server', 'infra/k8s/base', 'infra/terraform', 'deploy/local',
    'scripts/ci', 'scripts/load', 'docker/postgres/init',
    'docs/operations/runbook.md', 'docs/development/benchmarks-go-microbench.md',
    'docs/development/benchmarks-k6-room-slo.md'
) | ForEach-Object { Assert-Exists $_ }

$serverGo = @(Get-ChildItem 'backend/cmd/server/*.go' -ErrorAction SilentlyContinue)
if ($serverGo.Count -ne 1 -or $serverGo[0].Name -ne 'main.go') {
    Write-Host 'backend/cmd/server must contain only main.go' -ForegroundColor Red
    $script:fail = 1
}

$rulesCm = Get-Content 'deploy/alertmanager/rules-configmap.yaml' -Raw
if ($rulesCm -notmatch '(?m)^# Generated from deploy/alertmanager/rules.yml') {
    Write-Host 'deploy/alertmanager/rules-configmap.yaml must be generated (run: make sync-alert-rules)' -ForegroundColor Red
    $script:fail = 1
}

if ($script:fail -ne 0) { exit 1 }
Write-Output 'repo layout OK (ADR-021)'
