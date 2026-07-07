param(
    [string]$Path = ".",
    [switch]$Help
)

if ($Help) {
    Write-Host "Usage: ./scan-readability.ps1 [-Path <root>]"
    Write-Host "Scans code for readability and simplicity concerns."
    exit 0
}

$resPath = Resolve-Path $Path

Write-Host "=== Readability Scan ===" -ForegroundColor Cyan
Write-Host "Target: $resPath"
Write-Host ""

# 1. Large Go files (>500 lines)
Write-Host "[1/6] Large Go files (>500 lines)" -ForegroundColor Yellow
$goFiles = Get-ChildItem -Path "$resPath" -Recurse -Filter "*.go" -File
$largeFiles = $goFiles | Where-Object { (Get-Content $_.FullName | Measure-Object -Line).Lines -gt 500 }
if ($largeFiles) { $largeFiles | ForEach-Object { 
    $lines = (Get-Content $_.FullName | Measure-Object -Line).Lines
    Write-Host "  $($_.FullName.Replace($resPath,'')) — $lines lines" -ForegroundColor Red 
} }
else { Write-Host "  None found" -ForegroundColor Green }

# 2. Large TS files (>400 lines)
Write-Host "[2/6] Large TypeScript files (>400 lines)" -ForegroundColor Yellow
$tsFiles = Get-ChildItem -Path "$resPath" -Recurse -Include "*.ts" -File
$largeTsFiles = $tsFiles | Where-Object { (Get-Content $_.FullName | Measure-Object -Line).Lines -gt 400 }
if ($largeTsFiles) { $largeTsFiles | ForEach-Object { 
    $lines = (Get-Content $_.FullName | Measure-Object -Line).Lines
    Write-Host "  $($_.FullName.Replace($resPath,'')) — $lines lines" -ForegroundColor Red 
} }
else { Write-Host "  None found" -ForegroundColor Green }

# 3. Magic numbers (4+ digit literals)
Write-Host "[3/6] Magic number candidates (4+ digit literals)" -ForegroundColor Yellow
$magic = Select-String -Path "$resPath/**/*.go", "$resPath/**/*.ts" -Pattern "[^a-zA-Z0-9_][0-9]{4,}[^a-zA-Z0-9_]" -CaseSensitive
if ($magic) { $magic | ForEach-Object { Write-Host "  $($_.Path):$($_.LineNumber) — $($_.Line.Trim())" -ForegroundColor Yellow } }
else { Write-Host "  None found" -ForegroundColor Green }

# 4. Package-level variables (mutable state)
Write-Host "[4/6] Package-level variable declarations" -ForegroundColor Yellow
$pkgVars = Select-String -Path "$resPath/**/*.go" -Pattern "^\s*var\s+\w+\s+" -CaseSensitive -SimpleMatch
if ($pkgVars) { $pkgVars | ForEach-Object { 
    $ctx = Get-Content $_.Path | Select-Object -First 20
    $inFunc = $false
    foreach ($line in $ctx) {
        if ($line -match "^\s*func ") { $inFunc = $true }
    }
    if (-not $inFunc) {
        Write-Host "  $($_.Path):$($_.LineNumber) — $($_.Line.Trim())" -ForegroundColor Yellow
    }
} }
else { Write-Host "  None found" -ForegroundColor Green }

# 5. any usage in TypeScript
Write-Host "[5/6] 'any' type usage in TypeScript" -ForegroundColor Yellow
$anyUsage = Select-String -Path "$resPath/**/*.ts" -Pattern ":\s*any[^a-zA-Z]" -CaseSensitive
if ($anyUsage) { $anyUsage | ForEach-Object { Write-Host "  $($_.Path):$($_.LineNumber) — $($_.Line.Trim())" -ForegroundColor Red } }
else { Write-Host "  None found" -ForegroundColor Green }

# 6. Empty catch blocks / commented code patterns
Write-Host "[6/6] Commented-out code blocks" -ForegroundColor Yellow
$commented = Select-String -Path "$resPath/**/*.go", "$resPath/**/*.ts" -Pattern "//.*(deleted|removed|commented|commented out|legacy)" -CaseSensitive
if ($commented) { $commented | ForEach-Object { Write-Host "  $($_.Path):$($_.LineNumber) — $($_.Line.Trim())" -ForegroundColor Yellow } }
else { Write-Host "  None found" -ForegroundColor Green }

Write-Host "`n=== Scan Complete ===" -ForegroundColor Cyan
