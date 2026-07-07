param(
    [string]$Path = ".",
    [switch]$Help
)

if ($Help) {
    Write-Host "Usage: ./scan-security.ps1 [-Path <root>]"
    Write-Host "Scans code for security vulnerabilities and anti-patterns."
    exit 0
}

$resPath = Resolve-Path $Path
function Get-Files($Include) {
    Get-ChildItem -Path $resPath -Recurse -Include $Include -File
}

Write-Host "=== Security Scan ===" -ForegroundColor Cyan
Write-Host "Target: $resPath"
Write-Host ""

# 1. SQL injection risk — fmt.Sprintf with SQL keywords
Write-Host "[1/8] SQL injection risk (Sprintf + SQL)" -ForegroundColor Yellow
$goFiles = Get-Files "*.go"
$sqlInject = $goFiles | Select-String -Pattern "fmt\.Sprintf.*(SELECT|INSERT|UPDATE|DELETE|DROP|CREATE|ALTER)" -CaseSensitive
if ($sqlInject) { $sqlInject | ForEach-Object { Write-Host "  $($_.Path):$($_.LineNumber) — $($_.Line.Trim())" -ForegroundColor Red } }
else { Write-Host "  None found — parameterized queries likely in use" -ForegroundColor Green }

# 2. Hard-coded credential patterns
Write-Host "[2/8] Hard-coded credential patterns" -ForegroundColor Yellow
$allFiles = Get-Files @("*.go", "*.ts", "*.yaml", "*.yml", "*.json")
$credPat1 = $allFiles | Select-String -Pattern "password\s*[=:]\s*[""'][^""']{4,}[""']" -CaseSensitive
$credPat2 = $allFiles | Select-String -Pattern "secret\s*[=:]\s*[""'][^""']{8,}[""']" -CaseSensitive
$credPat3 = $allFiles | Select-String -Pattern "api[_-]?key\s*[=:]\s*[""'][^""']{8,}[""']" -CaseSensitive
$credPat4 = $allFiles | Select-String -Pattern "token\s*[=:]\s*[""'][^""']{8,}[""']" -CaseSensitive
$allCreds = @($credPat1, $credPat2, $credPat3, $credPat4) | Where-Object { $_ }
if ($allCreds) { $allCreds | ForEach-Object { Write-Host "  $($_.Path):$($_.LineNumber) — [POTENTIAL CREDENTIAL: $($_.Line.Trim())]" -ForegroundColor Red } }
else { Write-Host "  None found" -ForegroundColor Green }
Write-Host "  (gitleaks runs separately for comprehensive secret detection)" -ForegroundColor Gray

# 3. Plain HTTP URLs (non-TLS)
Write-Host "[3/8] Plain HTTP connections" -ForegroundColor Yellow
$httpUrls = $goFiles + (Get-Files "*.ts") | Select-String -Pattern '"http://' -CaseSensitive -SimpleMatch
if ($httpUrls) { $httpUrls | ForEach-Object {
    $line = $_.Line.Trim()
    if ($line -notmatch "//.*http://" -and $line -notmatch "localhost" -and $line -notmatch "//localhost") {
        Write-Host "  $($_.Path):$($_.LineNumber) — $line" -ForegroundColor Red
    }
} }
else { Write-Host "  None found" -ForegroundColor Green }

# 4. Constant-time comparison check
Write-Host "[4/8] Cryptographic comparison patterns" -ForegroundColor Yellow
$hmacEq = $allFiles | Select-String -Pattern "hmac\.Equal" -CaseSensitive -SimpleMatch
if ($hmacEq) { $hmacEq | ForEach-Object { Write-Host "  $($_.Path):$($_.LineNumber) — hmac.Equal (good)" -ForegroundColor Green } }
else { Write-Host "  No hmac.Equal found — verify comparison method" -ForegroundColor Yellow }

# 5. JWT handling
Write-Host "[5/8] JWT usage" -ForegroundColor Yellow
$jwtUsage = $goFiles | Select-String -Pattern "jwt\." -CaseSensitive -SimpleMatch
if ($jwtUsage) { $jwtUsage | ForEach-Object { Write-Host "  $($_.Path):$($_.LineNumber)" -ForegroundColor Gray } }
else { Write-Host "  None found" -ForegroundColor Gray }

# 6. Input validation at boundaries
Write-Host "[6/8] SQL parameterization check" -ForegroundColor Yellow
$sqlParams = $goFiles | Select-String -Pattern '\$1|pgx\.NamedArgs|sqlx\.Named' -CaseSensitive -SimpleMatch
if ($sqlParams) { Write-Host "  Parameterized queries confirmed — $($sqlParams.Count) matches" -ForegroundColor Green }
else { Write-Host "  No parameterized query patterns found — manual verification needed" -ForegroundColor Red }

# 7. TLS configuration
Write-Host "[7/8] TLS/SSL configuration" -ForegroundColor Yellow
$tlsConfig = ($goFiles + (Get-Files "*.ts")) | Select-String -Pattern "TLS|tls\.Config|InsecureSkipVerify" -CaseSensitive
if ($tlsConfig) { $tlsConfig | ForEach-Object {
    if ($_.Line -match "InsecureSkipVerify.*true") {
        Write-Host "  $($_.Path):$($_.LineNumber) — InsecureSkipVerify: true (RISK)" -ForegroundColor Red
    } else {
        Write-Host "  $($_.Path):$($_.LineNumber) — $($_.Line.Trim())" -ForegroundColor Gray
    }
} }
else { Write-Host "  None found" -ForegroundColor Yellow }

# 8. Debug/development endpoints
Write-Host "[8/8] Debug/info endpoints" -ForegroundColor Yellow
$debugEndpoints = $goFiles | Select-String -Pattern "(/debug|/pprof|/metrics|/health)" -CaseSensitive -SimpleMatch
if ($debugEndpoints) { $debugEndpoints | ForEach-Object { Write-Host "  $($_.Path):$($_.LineNumber) — $($_.Line.Trim())" -ForegroundColor Yellow } }
else { Write-Host "  None found" -ForegroundColor Green }

Write-Host "`n=== Scan Complete ===" -ForegroundColor Cyan
