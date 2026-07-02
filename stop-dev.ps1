Write-Host "=== Stopping dev servers ==="

# Kill by port
$ports = @(8080, 5173, 52175, 57266, 6060)
foreach ($p in $ports) {
    $conn = netstat -ano | findstr ":$p " | findstr LISTENING
    if ($conn) {
        $procId = ($conn -split '\s+')[-1]
        if ($procId -match '^\d+$') {
            Stop-Process -Id $procId -Force -ErrorAction SilentlyContinue
            Write-Host "  Killed PID $procId (port $p)"
        }
    }
}

# Kill leftover go/node processes from dev
Get-Process -Name "go" -ErrorAction SilentlyContinue | Where-Object { $_.StartInfo.FileName -like "*uppy*" -or $_.CommandLine -like "*server*" } | Stop-Process -Force -ErrorAction SilentlyContinue
Get-Process -Name "node" -ErrorAction SilentlyContinue | Where-Object { $_.MainWindowTitle -eq "" } | Stop-Process -Force -ErrorAction SilentlyContinue

Write-Host "=== Dev servers stopped ==="
