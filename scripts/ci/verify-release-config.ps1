# Pre-release static verification (no cloud access required).
# Usage: powershell -File scripts/ci/verify-release-config.ps1
$ErrorActionPreference = 'Stop'
$Root = Resolve-Path (Join-Path $PSScriptRoot '../..')
Set-Location $Root
$fail = 0

function Assert-Contains($path, $pattern, $msg) {
    $text = Get-Content $path -Raw
    if ($text -notmatch $pattern) {
        Write-Host "FAIL: $msg ($path)" -ForegroundColor Red
        $script:fail = 1
    } else {
        Write-Host "OK: $msg"
    }
}

Assert-Contains '.github/workflows/go-ci.yml' 'github\.sha' 'build-push uses commit SHA tag'
Assert-Contains '.github/workflows/go-ci.yml' 'cosign sign' 'build-push signs image with cosign'
Assert-Contains '.github/workflows/ci-cd.yml' 'cosign verify' 'deploy verifies cosign signature'
Assert-Contains '.github/workflows/ci-cd.yml' '__IMAGE_TAG__' 'deploy substitutes pinned image tag'
Assert-Contains '.github/workflows/ci-cd.yml' '__TRUSTED_PROXY_CIDRS__' 'deploy substitutes trusted proxy CIDRs'

foreach ($region in @('us-east1', 'europe-west1', 'asia-southeast1')) {
    $kust = "infra/k8s/overlays/$region/kustomization.yaml"
    if (-not (Test-Path $kust)) {
        Write-Host "FAIL: missing $kust" -ForegroundColor Red
        $fail = 1
    } elseif ((Get-Content $kust -Raw) -notmatch '__IMAGE_TAG__') {
        Write-Host "FAIL: $kust missing __IMAGE_TAG__ placeholder" -ForegroundColor Red
        $fail = 1
    } elseif ((Get-Content $kust -Raw) -notmatch '__TRUSTED_PROXY_CIDRS__') {
        Write-Host "FAIL: $kust missing __TRUSTED_PROXY_CIDRS__ placeholder" -ForegroundColor Red
        $fail = 1
    } else {
        Write-Host "OK: $kust has __IMAGE_TAG__ and __TRUSTED_PROXY_CIDRS__"
    }
}

if ($fail -ne 0) { exit 1 }
Write-Host 'Release config verification passed.'
