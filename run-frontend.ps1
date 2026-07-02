$root = Split-Path -Parent $MyInvocation.MyCommand.Path
. "$root\_bootstrap-env.ps1"
Set-Location "$root\frontend"
npm run dev
