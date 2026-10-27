# Builds dist\osureq.exe: a single Windows x64 executable with the dashboard embedded.
# Usage:  powershell -ExecutionPolicy Bypass -File build.ps1 [-Version 1.0.0] [-SkipTests]
param(
  [string]$Version = "1.0.0",
  [switch]$SkipTests
)
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
  $go = "C:\Program Files\Go\bin"
  if (Test-Path "$go\go.exe") { $env:Path = "$go;$env:Path" } else { throw "Go is not installed (winget install GoLang.Go)" }
}

if (-not $SkipTests) {
  go vet ./...
  if ($LASTEXITCODE) { exit $LASTEXITCODE }
  go test ./...
  if ($LASTEXITCODE) { exit $LASTEXITCODE }
}

$env:GOOS = "windows"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
New-Item -ItemType Directory -Force dist | Out-Null
go build -trimpath -ldflags "-s -w -X main.version=$Version" -o dist\osureq.exe .
if ($LASTEXITCODE) { exit $LASTEXITCODE }

$size = (Get-Item dist\osureq.exe).Length / 1MB
Write-Host ("Built dist\osureq.exe ({0:N1} MB)" -f $size)
