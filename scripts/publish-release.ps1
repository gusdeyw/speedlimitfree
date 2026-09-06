$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$taskOutput = Join-Path $taskRoot 'build/release'
$taskRelease = Get-Content -LiteralPath (Join-Path $taskOutput 'release.json') -Raw | ConvertFrom-Json
if ($taskRelease.version -notmatch '^\d+\.\d+\.\d+$' -or $taskRelease.commit -notmatch '^[a-f0-9]{40}$') { throw 'Invalid release metadata.' }
if ($taskRelease.repository -ne $env:GITHUB_REPOSITORY -or $taskRelease.commit -ne $env:GITHUB_SHA) { throw 'Release does not belong to this workflow commit/repository.' }
$taskRepo = $taskRelease.repository
$taskTag = 'v' + $taskRelease.version
$taskAssets = @('SpeedLimitFree-Setup.exe','SpeedLimitFree-windows-x64.zip','SHA256SUMS.txt') | ForEach-Object { Join-Path $taskOutput $_ }
foreach ($taskFile in $taskAssets) { if (-not (Test-Path -LiteralPath $taskFile -PathType Leaf)) { throw "Missing asset: $taskFile" } }
foreach ($taskLine in (Get-Content -LiteralPath (Join-Path $taskOutput 'SHA256SUMS.txt'))) {
    if ($taskLine -notmatch '^([a-f0-9]{64})  (SpeedLimitFree-Setup\.exe|SpeedLimitFree-windows-x64\.zip)$') { throw 'Invalid checksum entry.' }
    if ((Get-FileHash -LiteralPath (Join-Path $taskOutput $Matches[2]) -Algorithm SHA256).Hash -ne $Matches[1]) { throw 'Release checksum mismatch.' }
}
$taskExistingJSON = gh release list --repo $taskRepo --limit 1000 --json tagName,isDraft
if ($LASTEXITCODE) { throw 'Could not check existing releases.' }
$taskExisting = @($taskExistingJSON | ConvertFrom-Json) | Where-Object tagName -EQ $taskTag
if ($taskExisting -and -not $taskExisting.isDraft) {
    Write-Output "$taskTag is already published. Published assets are immutable; reruns leave them intact."
    exit 0
}
if ($taskExisting) {
    $taskDraftJSON = gh release view $taskTag --repo $taskRepo --json targetCommitish
    if ($LASTEXITCODE) { throw 'Could not check the draft commit.' }
    if (($taskDraftJSON | ConvertFrom-Json).targetCommitish -ne $taskRelease.commit) { throw 'Existing draft points to another commit.' }
}
if (-not $taskExisting) {
    gh release create $taskTag --repo $taskRepo --target $taskRelease.commit --title "SpeedLimitFree $($taskRelease.version)" --notes-file (Join-Path $taskOutput 'release-notes.md') --draft
    if ($LASTEXITCODE) { throw 'Draft release creation failed.' }
}
gh release upload $taskTag @taskAssets --repo $taskRepo --clobber
if ($LASTEXITCODE) { throw 'Asset upload failed. Release remains a draft.' }
gh release edit $taskTag --repo $taskRepo --notes-file (Join-Path $taskOutput 'release-notes.md') --draft=false --latest
if ($LASTEXITCODE) { throw 'Release publication failed.' }
$taskURL = "https://github.com/$taskRepo/releases/tag/$taskTag"
if ($env:GITHUB_STEP_SUMMARY) { Add-Content -LiteralPath $env:GITHUB_STEP_SUMMARY -Value "Published [$taskTag]($taskURL) with installer, ZIP, and checksums." }
Write-Output $taskURL
