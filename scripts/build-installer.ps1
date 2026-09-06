param([string]$Makensis)
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$taskTools = Join-Path $taskRoot '.tools'
New-Item -ItemType Directory -Path $taskTools -Force | Out-Null
if (-not $Makensis) {
    $Makensis = Join-Path $taskTools 'nsis-3.12\makensis.exe'
    if (-not (Test-Path -LiteralPath $Makensis)) {
        $taskZip = Join-Path $taskTools 'nsis-3.12.zip'
        & curl.exe --fail --location --silent --show-error --proto '=https' --proto-redir '=https' 'https://downloads.sourceforge.net/project/nsis/NSIS%203/3.12/nsis-3.12.zip' --output $taskZip
        if ($LASTEXITCODE) { throw 'NSIS download failed.' }
        if ((Get-FileHash -LiteralPath $taskZip -Algorithm SHA256).Hash -ne '56581F90DB321581C5381193D796FFFCF2D24B2F8FED2160A6C6A3BAA67F2C4F') { throw 'NSIS archive checksum mismatch.' }
        Expand-Archive -LiteralPath $taskZip -DestinationPath $taskTools -Force
    }
}
$taskBootstrap = Join-Path $taskTools 'MicrosoftEdgeWebview2Setup.exe'
if (-not (Test-Path -LiteralPath $taskBootstrap)) {
    & curl.exe --fail --location --silent --show-error --proto '=https' --proto-redir '=https' 'https://go.microsoft.com/fwlink/p/?LinkId=2124703' --output $taskBootstrap
    if ($LASTEXITCODE) { throw 'WebView2 bootstrapper download failed.' }
}
$taskSignature = Get-AuthenticodeSignature -LiteralPath $taskBootstrap
if ($taskSignature.Status -ne 'Valid' -or $taskSignature.SignerCertificate.Subject -notlike '*O=Microsoft Corporation*') { throw 'Microsoft bootstrapper signature verification failed.' }
$taskVersion = (Get-Content -LiteralPath (Join-Path $taskRoot 'wails.json') -Raw | ConvertFrom-Json).info.productVersion
if ($taskVersion -notmatch '^\d+\.\d+\.\d+$') { throw 'Invalid product version.' }
$taskPayload = Join-Path $taskRoot 'build\bin'
$taskOutput = Join-Path $taskPayload ("SpeedLimitFree-$taskVersion-Setup.exe")
& $Makensis /V2 "/DVERSION=$taskVersion" "/DPAYLOAD=$taskPayload" "/DBOOTSTRAPPER=$taskBootstrap" "/DOUTPUT=$taskOutput" (Join-Path $taskRoot 'build\windows\installer\project.nsi')
if ($LASTEXITCODE) { throw 'NSIS installer build failed.' }
Write-Output "Installer: $taskOutput"
Get-FileHash -LiteralPath $taskOutput -Algorithm SHA256 | Select-Object Hash
