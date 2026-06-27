$envFile = "D:\Project\多人网页游戏\.env"
Get-Content $envFile | ForEach-Object {
    if ($_ -match '^\s*([^#][^=]+)=(.*)\s*$') {
        $key = $matches[1].Trim()
        $val = $matches[2].Trim()
        Set-Item -Path "env:$key" -Value $val
    }
}
Set-Location D:\Project\多人网页游戏\backend
go run ./cmd/server
