#!/usr/bin/env pwsh
[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('build', 'dev', 'clean', 'tidy', 'vendor')]
    [string]$Target = 'build'
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$bin  = Join-Path $root 'conduit.exe'

function Invoke-UIInstall {
    Push-Location (Join-Path $root 'ui')
    try {
        if (-not (Test-Path 'node_modules')) {
            Write-Host '> npm install' -ForegroundColor Cyan
            npm install --no-audit --no-fund
            if ($LASTEXITCODE -ne 0) { throw "npm install failed" }
        }
    } finally { Pop-Location }
}

function Invoke-UIBuild {
    Push-Location (Join-Path $root 'ui')
    try {
        Write-Host '> npm run build' -ForegroundColor Cyan
        npm run build
        if ($LASTEXITCODE -ne 0) { throw "npm run build failed" }
    } finally { Pop-Location }
}

function Invoke-GoBuild {
    Push-Location $root
    try {
        Write-Host '> go build' -ForegroundColor Cyan
        go build -trimpath -ldflags '-s -w' -o $bin .
        if ($LASTEXITCODE -ne 0) { throw "go build failed" }
    } finally { Pop-Location }
}

switch ($Target) {
    'build' {
        Invoke-UIInstall
        Invoke-UIBuild
        Invoke-GoBuild
        Write-Host "Built $bin" -ForegroundColor Green
    }
    'dev' {
        Invoke-UIInstall
        Write-Host 'Starting Vite dev server and Go server (Ctrl+C to stop)...' -ForegroundColor Cyan
        $vite = Start-Process -PassThru -NoNewWindow pwsh -ArgumentList @(
            '-NoProfile', '-Command',
            "Set-Location '$(Join-Path $root "ui")'; npm run dev"
        )
        try {
            Push-Location $root
            go run .
        } finally {
            Pop-Location
            if ($vite -and -not $vite.HasExited) {
                Stop-Process -Id $vite.Id -Force -ErrorAction SilentlyContinue
            }
        }
    }
    'tidy' {
        Push-Location $root
        try { go mod tidy } finally { Pop-Location }
    }
    'vendor' {
        Push-Location $root
        try {
            go mod tidy
            go mod vendor
        } finally { Pop-Location }
    }
    'clean' {
        Remove-Item -Force -ErrorAction SilentlyContinue $bin
        Remove-Item -Recurse -Force -ErrorAction SilentlyContinue (Join-Path $root 'ui/dist')
        Remove-Item -Recurse -Force -ErrorAction SilentlyContinue (Join-Path $root 'ui/node_modules')
        Write-Host 'Cleaned' -ForegroundColor Yellow
    }
}
