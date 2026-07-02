$root = Split-Path -Parent $MyInvocation.MyCommand.Path
Get-Content "$root\.env" | ForEach-Object {
  if ($_ -match '^\s*([^#][^=]+)=(.*)\s*$') {
    [Environment]::SetEnvironmentVariable($matches[1].Trim(), $matches[2].Trim().Trim('"'))
  }
}
# MIGRATIONS_DIR is relative to the backend dir where go runs
$env:MIGRATIONS_DIR = "migrations"
Set-Location "$root\backend"
go run ./cmd/server
