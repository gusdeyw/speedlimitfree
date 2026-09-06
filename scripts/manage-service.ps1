param(
    [Parameter(Mandatory = $true)][ValidateSet('install','start','stop','restart')][string]$Action,
    [string]$OwnerSID,
    [ValidatePattern('^[a-f0-9]{32}$')][string]$OperationID
)
$ErrorActionPreference = 'Stop'
$taskResult = @{ operationId = $OperationID; success = $false; message = ''; action = $Action }
$taskData = Join-Path $env:ProgramData 'SpeedLimitFree'
$taskMutex = $null
$taskLocked = $false
$taskCanReport = $false
try {
    $taskIdentity = [Security.Principal.WindowsIdentity]::GetCurrent()
    if (-not ([Security.Principal.WindowsPrincipal]$taskIdentity).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Windows administrator access is required to manage the service.' }
    if (-not $OwnerSID) { $OwnerSID = $taskIdentity.User.Value }
    $taskOwner = New-Object Security.Principal.SecurityIdentifier($OwnerSID)
    . (Join-Path $PSScriptRoot 'service-common.ps1')
    $taskMutex = New-Object Threading.Mutex($false, 'Global\SpeedLimitFree.Setup')
    try { $taskLocked = $taskMutex.WaitOne(0) } catch [Threading.AbandonedMutexException] { $taskLocked = $true }
    if (-not $taskLocked) { throw 'Another service operation is already running. Wait for it to finish and refresh status.' }
    # Fixed protected location, never a caller-supplied elevated output path.
    Set-ProtectedDirectory $taskData $taskOwner
    $taskCanReport = $true
    switch ($Action) {
        'install' { $taskResult.message = Invoke-ServiceSetup $PSScriptRoot (Join-Path $env:ProgramFiles 'SpeedLimitFree') $taskData $taskOwner }
        'start' { Start-ManagedService; $taskResult.message = 'Service is running.' }
        'stop' { Stop-ManagedService; $taskResult.message = 'Service is stopped. Limits are inactive; saved rules are kept.' }
        'restart' { Stop-ManagedService; Start-ManagedService; $taskResult.message = 'Service restarted.' }
    }
    $taskResult.success = $true
} catch { $taskResult.message = $_.Exception.Message }
finally {
    if ($taskCanReport) {
        try {
            $taskLog = Join-Path $taskData 'setup.log'
            if ((Test-Path -LiteralPath $taskLog) -and (Get-Item -LiteralPath $taskLog).Length -gt 262144) { Move-Item -LiteralPath $taskLog -Destination (Join-Path $taskData 'setup.previous.log') -Force }
            Add-Content -LiteralPath $taskLog -Encoding UTF8 -Value ((Get-Date -Format o) + ' ' + $Action + ': ' + $taskResult.message)
            if ($OperationID) {
                $taskReports = Join-Path $taskData 'setup-results'
                Set-ProtectedDirectory $taskReports $taskOwner
                $taskResult | ConvertTo-Json -Compress | Set-Content -LiteralPath (Join-Path $taskReports ($OperationID + '.json')) -Encoding UTF8
                Get-ChildItem -LiteralPath $taskReports -Filter '*.json' | Sort-Object LastWriteTime -Descending | Select-Object -Skip 20 | ForEach-Object { Remove-Item -LiteralPath $_.FullName -Force }
            }
        } catch { $taskResult.success = $false; $taskResult.message += " Could not save setup result: $($_.Exception.Message)" }
    }
    if ($taskLocked) { $taskMutex.ReleaseMutex() }
    if ($taskMutex) { $taskMutex.Dispose() }
}
Write-Output $taskResult.message
if (-not $taskResult.success) { exit 1 }
