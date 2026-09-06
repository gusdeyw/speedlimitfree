$ErrorActionPreference = 'Stop'
# This command uses the same full-cleanup policy as Windows Installed Apps.
& (Join-Path $PSScriptRoot 'installer-actions.ps1') -Action uninstall
