# Verify branch protection required_status_checks match CI workflow job names.
# Usage: powershell -File scripts/ci/verify-required-checks.ps1
$ErrorActionPreference = 'Stop'
$Root = Resolve-Path (Join-Path $PSScriptRoot '../..')
Set-Location $Root

$settingsPath = Join-Path $Root '.github/settings.yml'
$settings = Get-Content $settingsPath -Raw

$required = [regex]::Matches($settings, '(?m)^\s+-\s+"([^"]+)"') |
    ForEach-Object { $_.Groups[1].Value } |
    Where-Object { $_ -notmatch '^(strict|contexts)$' }

# Job display names from ci-cd.yml and go-ci.yml (E2E matrix expands manually).
$ciJobs = @(
    'Quality Gate',
    'E2E Tests (gameplay)',
    'E2E Tests (performance)',
    'Test',
    'Lint',
    'Vet',
    'Integration Tests (testcontainers)',
    'Secret Scanning (detect-secrets)',
    'OpenAPI Validate',
    'Security Scan',
    'Secret Scanning (gitleaks)',
    'Container Scan',
    'Migration Test',
    'Docker Image Pinning',
    'License Check'
)

$missingInSettings = $ciJobs | Where-Object { $_ -notin $required }
$extraInSettings = $required | Where-Object { $_ -notin $ciJobs }

if ($missingInSettings.Count -gt 0) {
    Write-Host 'Required checks missing from settings.yml:' -ForegroundColor Red
    $missingInSettings | ForEach-Object { Write-Host "  - $_" }
}
if ($extraInSettings.Count -gt 0) {
    Write-Host 'settings.yml lists checks with no matching CI job:' -ForegroundColor Yellow
    $extraInSettings | ForEach-Object { Write-Host "  - $_" }
}

if ($missingInSettings.Count -eq 0 -and $extraInSettings.Count -eq 0) {
    Write-Host "OK: $($required.Count) required contexts align with CI job names."
    exit 0
}
exit 1
