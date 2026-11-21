# Verificación completa de DioptraEngine antes de un release.
# Uso:  .\scripts\check.ps1            (vet + gofmt + tests + build)
#       .\scripts\check.ps1 -Version v1.0.0 -Bench
param(
  [string]$Version = "dev",
  [switch]$Bench
)
$ErrorActionPreference = "Stop"
$prevCgo = $env:CGO_ENABLED
Push-Location (Join-Path $PSScriptRoot "..")
try {

Write-Host "== go vet" -ForegroundColor Cyan
go vet ./...
if ($LASTEXITCODE -ne 0) { exit 1 }

Write-Host "== gofmt" -ForegroundColor Cyan
$unformatted = gofmt -l cmd internal ui
if ($unformatted) { Write-Host "Archivos sin formato:`n$unformatted" -ForegroundColor Red; exit 1 }

Write-Host "== sintaxis de la UI (módulos ES)" -ForegroundColor Cyan
$uiFiles = @(Get-ChildItem ui/components/*.js | ForEach-Object { $_.FullName }) + @("ui/app.js", "ui/theme-init.js")
node --experimental-vm-modules --no-warnings scripts/check-ui.mjs @uiFiles
if ($LASTEXITCODE -ne 0) { exit 1 }

Write-Host "== go test" -ForegroundColor Cyan
go test ./...
if ($LASTEXITCODE -ne 0) { exit 1 }

if ($Bench) {
  Write-Host "== benchmarks (render)" -ForegroundColor Cyan
  go test ./internal/render -run '^$' -bench . -benchmem -count 3
}

Write-Host "== build $Version" -ForegroundColor Cyan
$env:CGO_ENABLED = "0"
New-Item -ItemType Directory -Force dist | Out-Null
go build -trimpath -ldflags "-s -w -X main.Version=$Version" -o dist/dioptra.exe ./cmd/dioptra
if ($LASTEXITCODE -ne 0) { exit 1 }
& ./dist/dioptra.exe --version
Write-Host "OK: dist/dioptra.exe" -ForegroundColor Green
} finally {
  if ($null -eq $prevCgo) {
    Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
  } else {
    $env:CGO_ENABLED = $prevCgo
  }
  Pop-Location
}
