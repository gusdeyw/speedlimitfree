# Real staging, hashing, file locks, and rollback; SCM and protected-directory ACLs are isolated fakes.
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'service-common.ps1')
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\.tools'))
$taskTestRoot = Join-Path $taskRoot ('setup-tests-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $taskTestRoot -Force | Out-Null
$taskOwner = [Security.Principal.WindowsIdentity]::GetCurrent().User
$script:caseNumber = 0
$script:checks = 0
function Assert([bool]$Condition, [string]$Message) { if (-not $Condition) { throw "ASSERT: $Message" }; $script:checks++ }
function Set-ProtectedDirectory([string]$Path, $Owner) { New-Item -ItemType Directory -Path $Path -Force | Out-Null }
function Set-ServiceExtras([string]$Install) {}
function Get-ManagedService { [pscustomobject]@{ Status = $script:serviceState; StartType = 'Automatic' } }
function Get-ServiceRegistration { [pscustomobject]@{ State = $script:serviceState; StartMode = 'Auto'; PathName = $script:binary; ProcessId = 0 } }
function Set-ServiceRegistration([string]$Binary, [string]$StartMode) { $script:binary = $Binary }
function Stop-Service { $script:stops++; $script:serviceState = 'Stopped' }
function Start-Service {
    $script:starts++
    if ($script:failNew -and [IO.File]::ReadAllText((Join-Path $script:install 'SpeedLimitFree-service.exe')) -eq 'new') { throw 'Simulated startup failure' }
    $script:serviceState = 'Running'
}
function Reset-Fixture {
    $script:caseNumber++
    $script:source = Join-Path $taskTestRoot ('source-' + $script:caseNumber)
    $script:install = Join-Path $taskTestRoot ('installed-' + $script:caseNumber)
    $script:data = Join-Path $taskTestRoot ('data-' + $script:caseNumber)
    foreach ($taskDir in @($script:source, $script:install, $script:data)) { New-Item -ItemType Directory -Path $taskDir | Out-Null }
    foreach ($taskFile in @('SpeedLimitFree.exe','SpeedLimitFree-service.exe','WinDivert.dll','WinDivert64.sys')) {
        [IO.File]::WriteAllText((Join-Path $script:source $taskFile), 'old')
        [IO.File]::WriteAllText((Join-Path $script:install $taskFile), 'old')
    }
    [IO.File]::WriteAllText((Join-Path $script:data 'rules.json'), 'saved rules must survive')
    $script:binary = '"' + (Join-Path $script:install 'SpeedLimitFree-service.exe') + '" --service --owner ' + $taskOwner.Value + ' --data "' + $script:data + '" --driver "' + $script:install + '"'
    $script:serviceState = 'Running'; $script:starts = 0; $script:stops = 0; $script:failNew = $false
}
function Setup { Invoke-ServiceSetup $script:source $script:install $script:data $taskOwner }
function Expect-Failure([scriptblock]$Action, [string]$Text) {
    $taskFailure = ''
    try { & $Action | Out-Null } catch { $taskFailure = $_.Exception.Message }
    Assert ($taskFailure.Contains($Text)) "Expected '$Text', got '$taskFailure'"
}
try {
    Reset-Fixture
    Setup | Out-Null
    Setup | Out-Null
    Assert ($script:stops -eq 0 -and $script:starts -eq 0) 'Repeated unchanged setup must not restart the service'
    Start-ManagedService
    Assert ($script:starts -eq 0) 'Start must be idempotent'
    Stop-ManagedService
    Stop-ManagedService
    Assert ($script:stops -eq 1) 'Stop must be idempotent'
    Start-ManagedService
    Assert ($script:starts -eq 1 -and $script:serviceState -eq 'Running') 'Stopped service starts'

    Reset-Fixture
    Remove-Item -LiteralPath (Join-Path $script:source 'WinDivert.dll')
    Expect-Failure { Setup } 'Missing WinDivert.dll'
    Assert ($script:stops -eq 0 -and $script:serviceState -eq 'Running') 'Incomplete package must not stop service'

    Reset-Fixture
    [IO.File]::WriteAllText((Join-Path $script:source 'SpeedLimitFree-service.exe'), 'new')
    [IO.File]::WriteAllText((Join-Path $script:source 'SpeedLimitFree.exe'), 'new desktop')
    $taskLock = [IO.File]::Open((Join-Path $script:install 'SpeedLimitFree.exe'), 'Open', 'Read', 'Read')
    try {
        $taskMessage = Setup
        Assert ($taskMessage -like '*could not be updated*') 'Locked desktop produces an actionable warning'
        Assert ($script:serviceState -eq 'Running') 'Locked desktop must not leave service stopped'
        Assert ([IO.File]::ReadAllText((Join-Path $script:install 'SpeedLimitFree-service.exe')) -eq 'new') 'Service updates despite a locked desktop'
    } finally { $taskLock.Dispose() }

    Reset-Fixture
    [IO.File]::WriteAllText((Join-Path $script:source 'SpeedLimitFree-service.exe'), 'new')
    $script:failNew = $true
    Expect-Failure { Setup } 'Previous service files and configuration were restored'
    Assert ([IO.File]::ReadAllText((Join-Path $script:install 'SpeedLimitFree-service.exe')) -eq 'old') 'Failed startup restores old executable'
    Assert ($script:serviceState -eq 'Running') 'Failed update restarts the old running service'
    Assert ([IO.File]::ReadAllText((Join-Path $script:data 'rules.json')) -eq 'saved rules must survive') 'Rollback preserves rules'

    Reset-Fixture
    [IO.File]::WriteAllText((Join-Path $script:source 'SpeedLimitFree-service.exe'), 'new')
    [IO.File]::WriteAllText((Join-Path $script:source 'WinDivert.dll'), 'new driver')
    $taskLock = [IO.File]::Open((Join-Path $script:install 'WinDivert.dll'), 'Open', 'Read', 'Read')
    try {
        Expect-Failure { Setup } 'Previous service files and configuration were restored'
        Assert ($script:serviceState -eq 'Running') 'Core copy failure restarts old service'
        Assert ([IO.File]::ReadAllText((Join-Path $script:install 'SpeedLimitFree-service.exe')) -eq 'old') 'Core copy failure restores already copied files'
    } finally { $taskLock.Dispose() }
    Write-Output "Service setup regression checks passed: $script:checks assertions across $script:caseNumber isolated fixtures."
} finally {
    $taskResolved = [IO.Path]::GetFullPath($taskTestRoot)
    if (-not $taskResolved.StartsWith($taskRoot.TrimEnd('\') + '\', [StringComparison]::OrdinalIgnoreCase) -or [IO.Path]::GetFileName($taskResolved) -notlike 'setup-tests-*') { throw 'Refusing test cleanup outside workspace tools.' }
    Remove-Item -LiteralPath $taskResolved -Recurse -Force
}
