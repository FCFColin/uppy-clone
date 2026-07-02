$root = Split-Path -Parent $MyInvocation.MyCommand.Path
Get-Content "$root\.env" | ForEach-Object {
  if ($_ -match '^\s*([^#][^=]+)=(.*)\s*$') {
    $key = $matches[1].Trim()
    $val = $matches[2].Trim().Trim('"')
    [Environment]::SetEnvironmentVariable($key, $val, "Process")
  }
}
