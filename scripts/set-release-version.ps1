param([Parameter(Mandatory)][ValidateRange(1,65536)][int]$BuildNumber)
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$taskConfigPath = Join-Path $taskRoot 'wails.json'
$taskConfig = Get-Content -LiteralPath $taskConfigPath -Raw | ConvertFrom-Json
$taskBase = [version]$taskConfig.info.productVersion
$taskPatch = $taskBase.Build + $BuildNumber - 1
if ($taskPatch -gt 65535) { throw 'Release patch exceeds the Windows version limit. Advance the base minor version.' }
$taskVersion = '{0}.{1}.{2}' -f $taskBase.Major,$taskBase.Minor,$taskPatch
$taskConfig.info.productVersion = $taskVersion
[IO.File]::WriteAllText($taskConfigPath, ($taskConfig | ConvertTo-Json -Depth 8) + "`n")
# npm handles the lockfile's empty root-package key, unsupported by Windows
# PowerShell 5.1 ConvertFrom-Json. Ignore lifecycle hooks while stamping metadata.
Push-Location (Join-Path $taskRoot 'frontend')
try {
    npm version $taskVersion --no-git-tag-version --allow-same-version --ignore-scripts
    if ($LASTEXITCODE) { throw 'Frontend version stamping failed.' }
} finally { Pop-Location }
$taskGuidePath = Join-Path $taskRoot 'build/START_HERE.md'
$taskGuide = [IO.File]::ReadAllText($taskGuidePath).Replace($taskBase.ToString(), $taskVersion)
[IO.File]::WriteAllText($taskGuidePath, $taskGuide)
if ($env:GITHUB_OUTPUT) { Add-Content -LiteralPath $env:GITHUB_OUTPUT -Value "version=$taskVersion" -Encoding utf8 }
Write-Output "Release version: $taskVersion"
