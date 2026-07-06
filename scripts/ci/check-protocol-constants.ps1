param (
    [string]$RepoRoot = (Resolve-Path "$PSScriptRoot/../..")
)

$ErrorActionPreference = "Stop"

# Go constants (source of truth)
$goFile = Join-Path $RepoRoot "backend/internal/constants/protocol.go"
$tsFile = Join-Path $RepoRoot "frontend/src/shared/game/protocol.ts"

# Parse Go file: extract const name = value as hex
$goConsts = @{}
Get-Content $goFile | Select-String -Pattern '^\s+(\w+)\s*=\s*(0x[0-9a-fA-F]+)' | ForEach-Object {
    $goConsts[$_.Matches.Groups[1].Value] = [int]$_.Matches.Groups[2].Value
}

# Parse TS file: extract name => hex values
$tsConsts = @{}
Get-Content $tsFile | Select-String -Pattern '(SNAPSHOT|PLAYER_JOIN|PLAYER_LEAVE|TAP_ACCEPTED|TAP_REJECTED|GAME_STATE_CHANGE|RESTART_STATUS|PONG|TAP|SET_NICKNAME|RESTART_VOTE|PING)\s*:\s*(0x[0-9a-fA-F]+)' | ForEach-Object {
    $name = $_.Matches.Groups[1].Value
    $value = [int]$_.Matches.Groups[2].Value
    $tsConsts[$name] = $value
}

# Build mapping: TS name -> Go name
$mapping = @{
    'SNAPSHOT'        = 'MsgSnapshot'
    'PLAYER_JOIN'     = 'MsgPlayerJoin'
    'PLAYER_LEAVE'    = 'MsgPlayerLeave'
    'TAP_ACCEPTED'    = 'MsgTapAccepted'
    'TAP_REJECTED'    = 'MsgTapRejected'
    'GAME_STATE_CHANGE' = 'MsgGameStateChange'
    'RESTART_STATUS'  = 'MsgRestartStatus'
    'PONG'            = 'MsgPong'
    'TAP'             = 'MsgTap'
    'SET_NICKNAME'    = 'MsgSetNickname'
    'RESTART_VOTE'    = 'MsgRestartVote'
    'PING'            = 'MsgPing'
}

$errors = @()
foreach ($tsName in $mapping.Keys) {
    $goName = $mapping[$tsName]
    $goVal = $goConsts[$goName]
    $tsVal = $tsConsts[$tsName]
    if ($null -eq $goVal) {
        $errors += "Go constant $goName not found"
        continue
    }
    if ($null -eq $tsVal) {
        $errors += "TS constant $tsName not found"
        continue
    }
    if ($goVal -ne $tsVal) {
        $errors += "MISMATCH: $goName=0x$('{0:x2}' -f $goVal) vs $tsName=0x$('{0:x2}' -f $tsVal)"
    }
}

if ($errors.Count -gt 0) {
    Write-Host "::error::Protocol constant mismatch between Go and TypeScript"
    $errors | ForEach-Object { Write-Host "  $_" }
    exit 1
}

Write-Host "✓ Protocol constants: Go and TypeScript are in sync ($($mapping.Count) values checked)"
