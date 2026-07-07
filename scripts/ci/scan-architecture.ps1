param(
    [string]$Path = ".",
    [switch]$Help
)

if ($Help) {
    Write-Host "Usage: ./scan-architecture.ps1 [-Path <root>]"
    Write-Host "Scans code for architecture and structural concerns."
    exit 0
}

$resPath = Resolve-Path $Path

Write-Host "=== Architecture Scan ===" -ForegroundColor Cyan
Write-Host "Target: $resPath"
Write-Host ""

# 1. init() functions
Write-Host "[1/6] init() functions" -ForegroundColor Yellow
$initFuncs = Select-String -Path "$resPath/**/*.go" -Pattern "^func init\(" -CaseSensitive -SimpleMatch
if ($initFuncs) { $initFuncs | ForEach-Object { Write-Host "  $($_.Path):$($_.LineNumber)" -ForegroundColor Red } }
else { Write-Host "  None found" -ForegroundColor Green }

# 2. Package directory sizes (total lines per package)
Write-Host "[2/6] Package size (total lines)" -ForegroundColor Yellow
$packages = Get-ChildItem -Path "$resPath" -Recurse -Directory | Where-Object { 
    $_.GetFiles("*.go").Count -gt 0 
}
$packages | ForEach-Object {
    $totalLines = ($_.GetFiles("*.go") | ForEach-Object { (Get-Content $_.FullName | Measure-Object -Line).Lines } | Measure-Object -Sum).Sum
    if ($totalLines -gt 1000) {
        Write-Host "  $($_.FullName.Replace($resPath,'')) — $totalLines total lines (over 1000)" -ForegroundColor Red
    }
    elseif ($totalLines -gt 500) {
        Write-Host "  $($_.FullName.Replace($resPath,'')) — $totalLines total lines" -ForegroundColor Yellow
    }
}

# 3. Cross-package dependency analysis (Go)
Write-Host "[3/6] Internal package import analysis" -ForegroundColor Yellow
$internalPkgs = Get-ChildItem -Path "$resPath" -Recurse -Directory | Where-Object { 
    $_.FullName -match "internal\\" -and $_.GetFiles("*.go").Count -gt 0
}
$internalPkgs | ForEach-Object {
    $pkgName = $_.FullName.Replace($resPath,'')
    $imports = Select-String -Path "$($_.FullName)\*.go" -Pattern '"github\.com/uppy-clone/backend/internal/\w+' -CaseSensitive
    if ($imports) {
        $externalImports = $imports | ForEach-Object { 
            if ($_.Line -match '"github\.com/uppy-clone/backend/internal/(\w+)') { $matches[1] }
        } | Sort-Object -Unique
        $externalImports | ForEach-Object { Write-Host "  $pkgName -> internal/$_" -ForegroundColor Gray }
    }
}

# 4. Large file count per package
Write-Host "[4/6] Packages with many source files (>10)" -ForegroundColor Yellow
$packages | ForEach-Object {
    $srcCount = ($_.GetFiles("*.go") | Where-Object { $_.Name -notmatch "_test" }).Count
    if ($srcCount -gt 10) {
        Write-Host "  $($_.FullName.Replace($resPath,'')) — $srcCount source files" -ForegroundColor Yellow
    }
}

# 5. Interface definitions
Write-Host "[5/6] Interface definitions" -ForegroundColor Yellow
$interfaces = Select-String -Path "$resPath/**/*.go" -Pattern "^type \w+ interface\s*\{" -CaseSensitive
if ($interfaces) { $interfaces | ForEach-Object { Write-Host "  $($_.Path):$($_.LineNumber) — $($_.Line.Trim())" -ForegroundColor Cyan } }
else { Write-Host "  None found" -ForegroundColor Green }

# 6. Global state patterns
Write-Host "[6/6] Singleton/global state patterns" -ForegroundColor Yellow
$globals = Select-String -Path "$resPath/**/*.go" -Pattern "var\s+\w+\s+\*" -CaseSensitive -SimpleMatch
if ($globals) { $globals | ForEach-Object { Write-Host "  $($_.Path):$($_.LineNumber) — $($_.Line.Trim())" -ForegroundColor Yellow } }
else { Write-Host "  None found" -ForegroundColor Green }

Write-Host "`n=== Scan Complete ===" -ForegroundColor Cyan
