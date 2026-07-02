$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$logFile = "$env:TEMP\uppy-clone-dev.log"

function Log { param($msg) $ts = Get-Date -Format "HH:mm:ss"; "$ts $msg" | Out-File -Append -LiteralPath $logFile; Write-Host $msg }

# ── 1. Kill old dev processes ──
Log "=== Killing old dev processes ==="
$oldPorts = @(8080, 5173, 52175, 57266, 6060)
foreach ($p in $oldPorts) {
    $conn = netstat -ano | findstr ":$p " | findstr LISTENING
    if ($conn) {
        $procId = ($conn -split '\s+')[-1]
        if ($procId -match '^\d+$') { Stop-Process -Id $procId -Force -ErrorAction SilentlyContinue }
    }
}
Start-Sleep -Seconds 1

# Kill leftover go/vite/node processes from previous runs
Get-Process -Name "go" -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Get-Process -Name "node" -ErrorAction SilentlyContinue | Where-Object { $_.MainWindowTitle -eq "" } | Stop-Process -Force -ErrorAction SilentlyContinue

# ── 2. Start Docker infrastructure ──
Log "=== Starting Docker infra: postgres + redis ==="
$dockerOk = $true
try {
    $composePs = docker compose ps --format json 2>&1 | Out-String
    if ($LASTEXITCODE -ne 0) { throw "Docker not running" }
    
    $needed = @("postgres", "redis")
    $raw = docker compose ps --services --filter "status=running" 2>&1
    $running = @($raw -split "`n" | ForEach-Object { $_.Trim() } | Where-Object { $_ -ne "" })
    foreach ($svc in $needed) {
        if ($running -notcontains $svc) {
            Log "  Starting $svc..."
            docker compose up -d $svc 2>&1 | Out-Null
            if ($LASTEXITCODE -ne 0) { throw "Failed to start $svc" }
        }
    }
} catch {
    Log "  WARNING: Docker unavailable ($($_.Exception.Message)). Assuming postgres/redis already running."
    $dockerOk = $false
}

# Wait for postgres on :5434 (TCP check, not HTTP)
$retries = 20
while ($retries -gt 0) {
    try { $tcp = [System.Net.Sockets.TcpClient]::new(); $tcp.ConnectAsync("127.0.0.1", 5434).Wait(1000); $ok = $tcp.Connected; $tcp.Dispose() } catch { $ok = $false }
    if ($ok) { break }
    Start-Sleep -Seconds 1; $retries--
}
if (-not $ok) { Log "  WARNING: Could not connect to postgres on :5434 after 20s" }
else { Log "  PostgreSQL ready on :5434" }

# ── 3. Load env ──
. "$root\_bootstrap-env.ps1"

# ── 4. Start backend (port 8080) ──
Log "=== Starting backend on :8080 ==="
$env:PORT = "8080"
$env:ALLOWED_ORIGINS = "http://localhost:5173"
$env:LOG_LEVEL = "info"
# Ensure ADMIN_JWT_SECRET and TRUSTED_PROXY_CIDRS are set (checked by server validation)
if (-not $env:ADMIN_JWT_SECRET) { $env:ADMIN_JWT_SECRET = $env:JWT_SECRET }
if (-not $env:TRUSTED_PROXY_CIDRS) { $env:TRUSTED_PROXY_CIDRS = "127.0.0.1/32" }

$backendCmd = ". '$root\_bootstrap-env.ps1'; " + `
    "`$env:PORT='8080'; " + `
    "`$env:ALLOWED_ORIGINS='http://localhost:5173'; " + `
    "`$env:ADMIN_JWT_SECRET='$($env:ADMIN_JWT_SECRET)'; " + `
    "`$env:TRUSTED_PROXY_CIDRS='$($env:TRUSTED_PROXY_CIDRS)'; " + `
    "`$env:MIGRATIONS_DIR='migrations'; " + `
    "Set-Location '$root\backend'; " + `
    "go run ./cmd/server"

Start-Process powershell -WindowStyle Hidden -ArgumentList "-NoProfile","-Command",$backendCmd

# ── 5. Start frontend (port 5173) ──
Log "=== Starting frontend on :5173 ==="
$frontendCmd = "Set-Location '$root\frontend'; npm run dev -- --port 5173"
Start-Process powershell -WindowStyle Hidden -ArgumentList "-NoProfile","-Command",$frontendCmd

# ── 6. Wait and verify ──
Log "=== Waiting for servers to be ready ==="
Start-Sleep -Seconds 12

$backendOk = $false; $frontendOk = $false
for ($i = 0; $i -lt 10; $i++) {
    try { $r = Invoke-WebRequest -Uri "http://localhost:8080/health" -UseBasicParsing -TimeoutSec 2; if ($r.StatusCode -eq 200) { $backendOk = $true } } catch {}
    try { $r = Invoke-WebRequest -Uri "http://localhost:5173/" -UseBasicParsing -TimeoutSec 2; if ($r.StatusCode -eq 200) { $frontendOk = $true } } catch {}
    if ($backendOk -and $frontendOk) { break }
    Start-Sleep -Seconds 2
}

Log ""
Log "=============================================="
Log "  Dev servers started"
if ($backendOk)  { Log "  Backend:  http://localhost:8080  [OK]" } else { Log "  Backend:  http://localhost:8080  [FAILED]" }
if ($frontendOk) { Log "  Frontend: http://localhost:5173  [OK]" } else { Log "  Frontend: http://localhost:5173  [FAILED]" }
Log "  Log:      $logFile"
Log "  Stop:     .\stop-dev.ps1"
Log "=============================================="

if (-not $backendOk -or -not $frontendOk) {
    Log "  Some servers failed. Check $logFile for details."
    Log "  Also check individual terminal windows (may have error messages)."
}
