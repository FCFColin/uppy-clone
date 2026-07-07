param(
    [string]$Path = ".",
    [switch]$Help
)

if ($Help) {
    Write-Host "Usage: ./scan-correctness.ps1 [-Path <root>]"
    Write-Host "Scans Go/TS code for correctness anti-patterns."
    exit 0
}

$resPath = Resolve-Path $Path

Write-Host "=== Correctness Scan ===" -ForegroundColor Cyan
Write-Host "Target: $resPath"
Write-Host ""

# 1. Swallowed errors: `if err != nil {}`
Write-Host "[1/7] Swallowed errors (if err != nil {})" -ForegroundColor Yellow
$swallowed = Select-String -Path "$resPath/**/*.go" -Pattern "if err != nil \{\s*\}" -CaseSensitive -SimpleMatch
if ($swallowed) { $swallowed | ForEach-Object { Write-Host "  $($_.Path):$($_.LineNumber)" -ForegroundColor Red } }
else { Write-Host "  None found" -ForegroundColor Green }

# 2. recover() usage
Write-Host "[2/7] recover() usage" -ForegroundColor Yellow
$recovers = Select-String -Path "$resPath/**/*.go" -Pattern "recover\(\)" -CaseSensitive -SimpleMatch
if ($recovers) { $recovers | ForEach-Object { Write-Host "  $($_.Path):$($_.LineNumber)" -ForegroundColor Yellow } }
else { Write-Host "  None found" -ForegroundColor Green }

# 3. fmt.Errorf without %w
Write-Host "[3/7] Errorf without %w" -ForegroundColor Yellow
$errNoWrap = Select-String -Path "$resPath/**/*.go" -Pattern 'fmt\.Errorf\("' -CaseSensitive -SimpleMatch
$errNoWrap = $errNoWrap | Where-Object { $_.Line -notmatch '%w' }
if ($errNoWrap) { $errNoWrap | ForEach-Object { Write-Host "  $($_.Path):$($_.LineNumber) — $($_.Line.Trim())" -ForegroundColor Red } }
else { Write-Host "  None found" -ForegroundColor Green }

# 4. Unchecked type assertions
Write-Host "[4/7] Unchecked type assertions" -ForegroundColor Yellow
$unchecked = Select-String -Path "$resPath/**/*.go" -Pattern '\.\((\w+)\)$' -CaseSensitive
if ($unchecked) { $unchecked | ForEach-Object { Write-Host "  $($_.Path):$($_.LineNumber) — $($_.Line.Trim())" -ForegroundColor Yellow } }
else { Write-Host "  None found" -ForegroundColor Green }

# 5. TODO/FIXME/HACK/XXX
Write-Host "[5/7] TODO/FIXME/HACK/XXX markers" -ForegroundColor Yellow
$todos = Select-String -Path "$resPath/**/*.go", "$resPath/**/*.ts" -Pattern "(TODO|FIXME|HACK|XXX|TEMP|WORKAROUND)" -CaseSensitive
if ($todos) { $todos | ForEach-Object { Write-Host "  $($_.Path):$($_.LineNumber) — $($_.Line.Trim())" -ForegroundColor Magenta } }
else { Write-Host "  None found" -ForegroundColor Green }

# 6. Log.Fatal / panic in non-main
Write-Host "[6/7] log.Fatal / os.Exit in non-main packages" -ForegroundColor Yellow
$fatals = Select-String -Path "$resPath/**/*.go" -Pattern "(log\.Fatal|os\.Exit)" -CaseSensitive -SimpleMatch
if ($fatals) { $fatals | ForEach-Object { Write-Host "  $($_.Path):$($_.LineNumber) — $($_.Line.Trim())" -ForegroundColor Red } }
else { Write-Host "  None found" -ForegroundColor Green }

# 7. defer in loops
Write-Host "[7/7] defer inside loops" -ForegroundColor Yellow
$deferLoops = Select-String -Path "$resPath/**/*.go" -Pattern "defer " -CaseSensitive -SimpleMatch
# Note: this is a rough check; manual verification needed
if ($deferLoops) { Write-Host "  $($deferLoops.Count) defer calls found — manual check required for loop context" -ForegroundColor Yellow }
else { Write-Host "  None found" -ForegroundColor Green }

Write-Host "`n=== Scan Complete ===" -ForegroundColor Cyan
