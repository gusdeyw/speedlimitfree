param([string]$OwnerSID)
$ErrorActionPreference = 'Stop'
& (Join-Path $PSScriptRoot 'manage-service.ps1') -Action install -OwnerSID $OwnerSID
