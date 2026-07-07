param(
    [string]$Path = ".",
    [switch]$Help
)

if ($Help) {
    Write-Host "Usage: ./scan-performance.ps1 [-Path <root>]"
    Write-Host "Scans code for potential performance bottlenecks."
    exit 0
}

$resPath = Resolve-Path $Path

Write-Host "=== Performance Scan ===" -ForegroundColor Cyan
Write-Host "Target: $resPath"
Write-Host ""

# 1. SELECT * queries
Write-Host "[1/7] SELECT * queries" -ForegroundColor Yellow
$selectStar = Select-String -Path "$resPath/**/*.go" -Pattern "SELECT \*" -CaseSensitive -SimpleMatch
if ($selectStar) { $selectStar | ForEach-Object { Write-Host "  $($_.Path):$($_.LineNumber) — $($_.Line.Trim())" -ForegroundColor Red } }
else { Write-Host "  None found" -ForegroundColor Green }

# 2. N+1 query pattern (in loops)
Write-Host "[2/7] N+1 query risk (DB calls in loops)" -ForegroundColor Yellow
$nplus1 = Select-String -Path "$resPath/**/*.go" -Pattern "for.*range" -CaseSensitive -SimpleMatch
Write-Host "  $($nplus1.Count) range loops found — manual check for DB/API calls inside loops needed" -ForegroundColor Yellow

# 3. Goroutine leak risk
Write-Host "[3/7] Goroutine creation" -ForegroundColor Yellow
$goRoutines = Select-String -Path "$resPath/**/*.go" -Pattern "go func\(" -CaseSensitive -SimpleMatch
if ($goRoutines) { 
    $goRoutines | ForEach-Object { Write-Host "  $($_.Path):$($_.LineNumber)" -ForegroundColor Yellow }
    Write-Host "  Total: $($goRoutines.Count) goroutines — verify WaitGroup/signal handling" -ForegroundColor Yellow
}
else { Write-Host "  None found" -ForegroundColor Green }

# 4. Unbounded loops
Write-Host "[4/7] Unbounded loops (for {})" -ForegroundColor Yellow
$unbounded = Select-String -Path "$resPath/**/*.go" -Pattern "for\s*\{\s*$" -CaseSensitive
if ($unbounded) { $unbounded | ForEach-Object { Write-Host "  $($_.Path):$($_.LineNumber) — $($_.Line.Trim())" -ForegroundColor Red } }
else { Write-Host "  None found" -ForegroundColor Green }

# 5. JSON marshaling in hot paths
Write-Host "[5/7] JSON marshal/unmarshal usage" -ForegroundColor Yellow
$jsonOps = Select-String -Path "$resPath/**/*.go", "$resPath/**/*.ts" -Pattern "(json\.Marshal|json\.Unmarshal|JSON\.stringify|JSON\.parse)" -CaseSensitive
if ($jsonOps) { 
    $jsonOps | ForEach-Object { Write-Host "  $($_.Path):$($_.LineNumber)" -ForegroundColor Gray }
    Write-Host "  Total: $($jsonOps.Count) JSON operations — verify hot path frequency" -ForegroundColor Yellow
}
else { Write-Host "  None found" -ForegroundColor Green }

# 6. Large allocations (make with large size)
Write-Host "[6/7] make() with large/dynamic sizes" -ForegroundColor Yellow
$makeAlloc = Select-String -Path "$resPath/**/*.go" -Pattern "make\(\[" -CaseSensitive -SimpleMatch
if ($makeAlloc) { $makeAlloc | ForEach-Object { Write-Host "  $($_.Path):$($_.LineNumber) — $($_.Line.Trim())" -ForegroundColor Yellow } }
else { Write-Host "  None found" -ForegroundColor Green }

# 7. Synchronous HTTP/WaitGroup usage
Write-Host "[7/7] sync.WaitGroup usage pattern" -ForegroundColor Yellow
$wgUsage = Select-String -Path "$resPath/**/*.go" -Pattern "WaitGroup" -CaseSensitive -SimpleMatch
if ($wgUsage) { $wgUsage | ForEach-Object { Write-Host "  $($_.Path):$($_.LineNumber)" -ForegroundColor Gray } }
else { Write-Host "  None found" -ForegroundColor Gray }

Write-Host "`n=== Scan Complete ===" -ForegroundColor Cyan
