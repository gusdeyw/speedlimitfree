param(
    [ValidatePattern('^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$')][string]$Repository = 'gusdeyw/speedlimitfree',
    [ValidatePattern('^[a-fA-F0-9]{40}$')][string]$Commit = (git rev-parse HEAD)
)
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$taskVersion = (Get-Content -LiteralPath (Join-Path $taskRoot 'wails.json') -Raw | ConvertFrom-Json).info.productVersion
if ($taskVersion -notmatch '^\d+\.\d+\.\d+$') { throw 'Invalid release version.' }
$taskBin = Join-Path $taskRoot 'build/bin'
$taskOutput = Join-Path $taskRoot 'build/release'
$taskStage = Join-Path $taskRoot ('.tools/release-' + [guid]::NewGuid().ToString('N'))
# Explicit allowlist: no historical binaries, test data, or local logs enter the ZIP.
. (Join-Path $PSScriptRoot 'installer-common.ps1')
$taskFiles = @($script:PayloadFiles | Where-Object { $_ -ne 'Uninstall.exe' })
foreach ($taskFile in $taskFiles) {
    if (-not (Test-Path -LiteralPath (Join-Path $taskBin $taskFile) -PathType Leaf)) { throw "Missing release payload: $taskFile" }
}
$taskInstaller = Join-Path $taskBin "SpeedLimitFree-$taskVersion-Setup.exe"
foreach ($taskExe in @($taskInstaller,(Join-Path $taskBin 'SpeedLimitFree.exe'))) {
    $taskInfo = (Get-Item -LiteralPath $taskExe).VersionInfo
    if ($taskInfo.ProductVersion -ne $taskVersion -or $taskInfo.CompanyName -ne 'gusdeyw') { throw "Release metadata mismatch: $taskExe" }
}
New-Item -ItemType Directory -Path $taskOutput,$taskStage -Force | Out-Null
try {
    $taskFolder = Join-Path $taskStage "SpeedLimitFree-$taskVersion-windows-x64"
    New-Item -ItemType Directory -Path $taskFolder | Out-Null
    foreach ($taskFile in $taskFiles) { Copy-Item -LiteralPath (Join-Path $taskBin $taskFile) -Destination $taskFolder }
    Copy-Item -LiteralPath $taskInstaller -Destination (Join-Path $taskOutput 'SpeedLimitFree-Setup.exe') -Force
    Compress-Archive -LiteralPath $taskFolder -DestinationPath (Join-Path $taskOutput 'SpeedLimitFree-windows-x64.zip') -Force
} finally { Remove-OwnedTree $taskStage (Join-Path $taskRoot '.tools') }
$taskChecksums = foreach ($taskAsset in @('SpeedLimitFree-Setup.exe','SpeedLimitFree-windows-x64.zip')) {
    $taskHash = (Get-FileHash -LiteralPath (Join-Path $taskOutput $taskAsset) -Algorithm SHA256).Hash.ToLowerInvariant()
    "$taskHash  $taskAsset"
}
[IO.File]::WriteAllLines((Join-Path $taskOutput 'SHA256SUMS.txt'), [string[]]$taskChecksums)
$taskURL = "https://github.com/$Repository/releases/download/v$taskVersion"
$taskNotes = @"
## Download for Windows 11 x64

| Download | Use it for |
| --- | --- |
| **[Download installer (.exe)]($taskURL/SpeedLimitFree-Setup.exe)** | Recommended. Installs the app, background service, and Start menu shortcut. |
| **[Download ZIP package]($taskURL/SpeedLimitFree-windows-x64.zip)** | Extract the whole folder, open SpeedLimitFree.exe, then use Settings > Install / repair. |
| [SHA-256 checksums]($taskURL/SHA256SUMS.txt) | Verify the downloaded files. |

Version **$taskVersion** | Created by [gusdeyw](https://github.com/gusdeyw).

### Included

- Searchable application table with separate download and upload limits.
- Windows system theme, executable icons, and close-to-tray behavior.
- Background service with Start, Stop, Restart, and Install / repair controls.
- Setup preserves saved rules during upgrades. Uninstall removes all app-owned rules, settings, logs, and caches.

### Installation notes

Run the installer and approve the administrator prompt. WebView2 is downloaded only when missing. The ZIP also requires service setup for traffic limiting; it is not a fully portable limiter.

This build is unsigned. Windows may show an unknown-publisher or SmartScreen prompt. Automated checks cover application behavior and isolated installer transactions; live traffic accuracy and elevated clean-machine installation remain under validation.

The Source code archives below are for developers. Choose the EXE or ZIP links above to run the app.

Built from [$Commit](https://github.com/$Repository/commit/$Commit). [Latest release](https://github.com/$Repository/releases/latest) · [Report an issue](https://github.com/$Repository/issues/new).
"@
[IO.File]::WriteAllText((Join-Path $taskOutput 'release-notes.md'), $taskNotes + "`n")
[IO.File]::WriteAllText((Join-Path $taskOutput 'release.json'), (@{version=$taskVersion;commit=$Commit;repository=$Repository} | ConvertTo-Json) + "`n")
Write-Output "Release assets and notes: $taskOutput"
