$root = Split-Path -Parent $MyInvocation.MyCommand.Path
. "$root\_bootstrap-env.ps1"
$env:MIGRATIONS_DIR = "migrations"
Set-Location "$root\backend"
go run ./cmd/server