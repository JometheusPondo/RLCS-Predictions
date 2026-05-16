# make.ps1 — PowerShell-native equivalent of the Makefile for Windows users
# who aren't running Git Bash. Usage: ./make.ps1 <target>
#
# Targets: dev | frontend | backend | build | clean

param(
    [Parameter(Position = 0)]
    [string]$Target = "help"
)

$ErrorActionPreference = "Stop"

switch ($Target) {
    "dev" {
        # Parallel processes inside one terminal get messy on Windows.
        # Cleanest path: two terminals. Print the commands.
        Write-Host "Run these in two separate terminals from the project root:"
        Write-Host ""
        Write-Host "  Terminal 1 (frontend):"
        Write-Host "    cd web; pnpm dev"
        Write-Host ""
        Write-Host "  Terminal 2 (backend):"
        Write-Host "    go run ./cmd/server"
    }
    "frontend" {
        Push-Location web
        try {
            pnpm install --frozen-lockfile
            pnpm run build
        }
        finally { Pop-Location }
    }
    "backend" {
        go build -o bin/server.exe ./cmd/server
    }
    "build" {
        Push-Location web
        try {
            pnpm install --frozen-lockfile
            pnpm run build
        }
        finally { Pop-Location }
        go build -o bin/server.exe ./cmd/server
    }
    "clean" {
        if (Test-Path web/dist) { Remove-Item -Recurse -Force web/dist }
        if (Test-Path bin)      { Remove-Item -Recurse -Force bin }
        # Recreate the embed placeholder so //go:embed all:web/dist still
        # compiles after a clean.
        New-Item -ItemType Directory -Force -Path web/dist | Out-Null
        New-Item -ItemType File -Force -Path web/dist/.gitkeep | Out-Null
    }
    default {
        Write-Host "Usage: ./make.ps1 {dev|frontend|backend|build|clean}"
    }
}
