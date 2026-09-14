param(
    [int]$Port = 8080,
    [string]$DatabasePath = "data/rlcs.db"
)

$ErrorActionPreference = "Stop"
Push-Location $PSScriptRoot
try {
    & ./make.ps1 build

    $env:PORT = [string]$Port
    $env:DATABASE_PATH = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot $DatabasePath))
    $env:ACTIVE_EVENT = "worlds-2026"
    Write-Host "Open http://localhost:$Port — use Event to switch between Worlds and the Major archive."
    & ./bin/server.exe
    if ($LASTEXITCODE -ne 0) { throw "Server exited with code $LASTEXITCODE" }
}
finally {
    Pop-Location
}
