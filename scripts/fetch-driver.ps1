param([string]$Destination = (Join-Path $PSScriptRoot '..\third_party\windivert'))
$ErrorActionPreference = 'Stop'
$Destination = [IO.Path]::GetFullPath($Destination)
$taskArchive = Join-Path $env:TEMP ('speedlimitfree-windivert-' + [guid]::NewGuid().ToString('N') + '.zip')
$taskExtract = Join-Path $env:TEMP ('speedlimitfree-windivert-' + [guid]::NewGuid().ToString('N'))
$taskURL = 'https://github.com/basil00/WinDivert/releases/download/v2.2.2/WinDivert-2.2.2-A.zip'
Invoke-WebRequest -UseBasicParsing -Uri $taskURL -OutFile $taskArchive
$taskHash = (Get-FileHash -LiteralPath $taskArchive -Algorithm SHA256).Hash
if ($taskHash -ne '63CB41763BB4B20F600B6DE04E991A9C2BE73279E317D4D82F237B150C5F3F15') { throw 'WinDivert archive checksum differs from the pinned distribution.' }
Expand-Archive -LiteralPath $taskArchive -DestinationPath $taskExtract
$taskSource = Join-Path $taskExtract 'WinDivert-2.2.2-A'
New-Item -ItemType Directory -Path $Destination -Force | Out-Null
foreach ($taskFile in @('WinDivert.dll', 'WinDivert64.sys')) {
    Copy-Item -LiteralPath (Join-Path $taskSource ('x64\' + $taskFile)) -Destination $Destination -Force
}
foreach ($taskFile in @('LICENSE', 'COPYING', 'COPYING.LESSER')) {
    if (Test-Path -LiteralPath (Join-Path $taskSource $taskFile)) { Copy-Item -LiteralPath (Join-Path $taskSource $taskFile) -Destination $Destination -Force }
}
@{ version = '2.2.2-A'; url = $taskURL; sha256 = $taskHash; source = 'https://github.com/basil00/WinDivert/tree/v2.2.2' } | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $Destination 'distribution.json') -Encoding UTF8
Get-AuthenticodeSignature -LiteralPath (Join-Path $Destination 'WinDivert64.sys') | Select-Object Status, StatusMessage
Write-Output "WinDivert downloaded to $Destination (SHA256 $taskHash)"
