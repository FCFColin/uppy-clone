# Cleanup script: Remove old Cloudflare Workers code
# Run this script after verifying the Go backend + Vite frontend works correctly
#
# WARNING: This is irreversible! Make sure you have a backup or git commit before running.

Write-Host "=== Old Code Cleanup Script ===" -ForegroundColor Yellow
Write-Host "This will remove the following:" -ForegroundColor Yellow

$itemsToDelete = @(
    # Old Workers backend
    "src",

    # Old client-side code (replaced by frontend/src/)
    "public\game.js",
    "public\game",
    "public\name-pools.js",
    "public\index.js",
    "public\admin.js",
    "public\index.html",
    "public\play.html",
    "public\admin.html",

    # Old Workers config
    "wrangler.jsonc",
    "worker-configuration.d.ts",

    # Old build scripts (now handled by Vite + Go)
    "scripts\generate-name-pools.mjs",
    "scripts\generate-assets.mjs",

    # Old test infrastructure (replaced by Go tests)
    "tests",

    # Old TypeScript config (now in frontend/)
    "tsconfig.json"
)

foreach ($item in $itemsToDelete) {
    $fullPath = Join-Path $PSScriptRoot "..\$item"
    if (Test-Path $fullPath) {
        Write-Host "  WILL DELETE: $item" -ForegroundColor Red
    } else {
        Write-Host "  NOT FOUND:   $item" -ForegroundColor Gray
    }
}

Write-Host ""
$confirm = Read-Host "Type 'DELETE' to confirm deletion"

if ($confirm -eq 'DELETE') {
    foreach ($item in $itemsToDelete) {
        $fullPath = Join-Path $PSScriptRoot "..\$item"
        if (Test-Path $fullPath) {
            Remove-Item -Path $fullPath -Recurse -Force
            Write-Host "  DELETED: $item" -ForegroundColor Green
        }
    }

    # Also update package.json to remove Cloudflare dependencies
    Write-Host ""
    Write-Host "Updating package.json..." -ForegroundColor Yellow

    # Read and modify package.json
    $packageJsonPath = Join-Path $PSScriptRoot "..\package.json"
    if (Test-Path $packageJsonPath) {
        $pkg = Get-Content $packageJsonPath | ConvertFrom-Json

        # Remove Cloudflare devDependencies
        $cfDeps = @(
            "@cloudflare/vitest-pool-workers",
            "@cloudflare/workers-types",
            "wrangler"
        )

        $newDevDeps = @{}
        foreach ($key in $pkg.devDependencies.PSObject.Properties.Name) {
            if ($key -notin $cfDeps) {
                $newDevDeps[$key] = $pkg.devDependencies.$key
            }
        }

        # Remove old scripts
        $scriptsToRemove = @("dev", "deploy", "db:migrate", "db:migrate:prod", "test:do")
        foreach ($s in $scriptsToRemove) {
            if ($pkg.scripts.PSObject.Properties.Name -contains $s) {
                $pkg.scripts.PSObject.Properties.Remove($s)
            }
        }

        # Add new scripts
        $pkg.scripts | Add-Member -NotePropertyName "dev:frontend" -NotePropertyValue "cd frontend && npm run dev" -Force
        $pkg.scripts | Add-Member -NotePropertyName "dev:backend" -NotePropertyValue "cd backend && go run ./cmd/server" -Force
        $pkg.scripts | Add-Member -NotePropertyName "build:frontend" -NotePropertyValue "cd frontend && npm run build" -Force
        $pkg.scripts | Add-Member -NotePropertyName "build:backend" -NotePropertyValue "cd backend && go build ./cmd/server" -Force
        $pkg.scripts | Add-Member -NotePropertyName "test:backend" -NotePropertyValue "cd backend && go test ./..." -Force
        $pkg.scripts | Add-Member -NotePropertyName "docker:up" -NotePropertyValue "docker-compose up --build" -Force
        $pkg.scripts | Add-Member -NotePropertyName "docker:down" -NotePropertyValue "docker-compose down" -Force

        $pkg.devDependencies = $newDevDeps
        $pkg | ConvertTo-Json -Depth 10 | Set-Content $packageJsonPath
        Write-Host "  package.json updated" -ForegroundColor Green
    }

    Write-Host ""
    Write-Host "Cleanup complete!" -ForegroundColor Green
} else {
    Write-Host "Cleanup cancelled." -ForegroundColor Yellow
}
