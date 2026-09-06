param([switch]$SkipDriver, [switch]$SkipTests, [switch]$SkipInstaller)
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
Set-Location -LiteralPath $taskRoot
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    if (Test-Path -LiteralPath 'D:\Go\bin\go.exe') { $env:PATH = 'D:\Go\bin;' + $env:PATH } else { throw 'Install Go 1.25 or newer and add it to PATH.' }
}
$taskGoBin = Join-Path (go env GOPATH) 'bin'
$env:PATH = $taskGoBin + ';' + $env:PATH
if (-not (Get-Command wails -ErrorAction SilentlyContinue)) { go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0; if ($LASTEXITCODE) { throw 'Wails installation failed.' } }
Push-Location frontend
try {
    npm ci
    if ($LASTEXITCODE) { throw 'Frontend dependency installation failed.' }
    npm run build
    if ($LASTEXITCODE) { throw 'Frontend build failed.' }
} finally { Pop-Location }
if (-not $SkipTests) {
    go test ./...
    if ($LASTEXITCODE) { throw 'Go tests failed.' }
    go vet ./...
    if ($LASTEXITCODE) { throw 'Go vet failed.' }
    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$PSScriptRoot\test-service-setup.ps1"
    if ($LASTEXITCODE) { throw 'Service setup regression checks failed.' }
    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$PSScriptRoot\test-installer.ps1"
    if ($LASTEXITCODE) { throw 'Installer transaction and cleanup checks failed.' }
}
# Wails only creates the Windows ICO when it is missing. Always derive it from
# the same PNG embedded by the tray so an older resource cannot survive a build.
$taskWindowsIcon = Join-Path $taskRoot 'build\windows\icon.ico'
if (Test-Path -LiteralPath $taskWindowsIcon) { Remove-Item -LiteralPath $taskWindowsIcon -Force }
wails build -s -platform windows/amd64
if ($LASTEXITCODE) { throw 'Desktop build failed.' }
go build -trimpath -o build/bin/SpeedLimitFree-service.exe ./cmd/service
if ($LASTEXITCODE) { throw 'Service build failed.' }
go build -trimpath -o build/bin/limiter-debug.exe ./cmd/limiter-debug
if ($LASTEXITCODE) { throw 'Diagnostic tool build failed.' }
if (-not $SkipDriver) {
    if (-not (Test-Path -LiteralPath 'third_party\windivert\WinDivert.dll')) { & "$PSScriptRoot\fetch-driver.ps1" }
    Copy-Item -Path 'third_party\windivert\*' -Destination 'build\bin' -Force
}
Copy-Item -LiteralPath "$PSScriptRoot\install-service.ps1" -Destination 'build\bin' -Force
Copy-Item -LiteralPath "$PSScriptRoot\manage-service.ps1" -Destination 'build\bin' -Force
Copy-Item -LiteralPath "$PSScriptRoot\service-common.ps1" -Destination 'build\bin' -Force
Copy-Item -LiteralPath "$PSScriptRoot\uninstall-service.ps1" -Destination 'build\bin' -Force
foreach ($taskHelper in @('installer-common.ps1','installer-actions.ps1','installer-native.cs')) { Copy-Item -LiteralPath (Join-Path $PSScriptRoot $taskHelper) -Destination 'build\bin' -Force }
Copy-Item -LiteralPath 'THIRD_PARTY_NOTICES.md' -Destination 'build\bin' -Force
Copy-Item -LiteralPath 'build\START_HERE.md' -Destination 'build\bin' -Force
if (-not $SkipInstaller) { & "$PSScriptRoot\build-installer.ps1" }
Write-Output "Built desktop, service, and diagnostic tool in $taskRoot\build\bin"
