# Automated subset of docs/security/self-check-checklist.md layers 2–6.
# Usage: powershell -File scripts/ci/self-check-layers.ps1
$ErrorActionPreference = 'Stop'
$Root = Resolve-Path (Join-Path $PSScriptRoot '../..')
Set-Location $Root

function Run-Step($label, [scriptblock]$block) {
    Write-Host "==> $label"
    Push-Location $Root
    try {
        & $block
        if ($LASTEXITCODE -ne 0) { throw "$label failed (exit $LASTEXITCODE)" }
    } finally {
        Pop-Location
    }
}

Write-Host 'Layer 2: Auth & session'
Run-Step 'auth package' { Set-Location backend; go test ./internal/auth/... -count=1 }
Run-Step 'auth handlers' { Set-Location backend; go test ./internal/handler/... -run 'Magic|QuickPlay|Admin|Logout|Refresh|Revoke' -count=1 }
Run-Step 'frontend auth/session' { Set-Location frontend; npx vitest run src/shared/network/auth.test.ts src/shared/network/session.test.ts }

Write-Host 'Layer 3: WebSocket & game'
Run-Step 'websocket handlers' { Set-Location backend; go test ./internal/handler/... -run 'WebSocket|WS' -count=1 }

Write-Host 'Layer 4: Input validation'
Run-Step 'nickname validate' { Set-Location backend; go test ./internal/validate/... -count=1 }

Write-Host 'Layer 5: Middleware & rate limits'
Run-Step 'ratelimit' { Set-Location backend; go test ./internal/middleware/... -run RateLimit -count=1 }

Write-Host 'Layer 6: Contracts'
Run-Step 'cooldown contract (Go)' { Set-Location backend; go test ./internal/game/... -run CooldownContract -count=1 }
Run-Step 'cooldown contract (TS)' { Set-Location frontend; npx vitest run src/game/cooldown_contract.test.ts }

Write-Host 'All automated layer checks passed.'
