$ErrorActionPreference = 'Stop'

function Get-ManagedService {
    $taskController = Get-Service -Name SpeedLimitFree -ErrorAction SilentlyContinue
    if ($taskController) {
        try { [pscustomobject]@{ Status=$taskController.Status; StartType=$taskController.StartType } }
        finally { $taskController.Dispose() }
    }
}

function Wait-ManagedService([string]$State) {
    $taskTimer = [Diagnostics.Stopwatch]::StartNew()
    do {
        $taskService = Get-ManagedService
        if (-not $taskService) { throw 'The service is not installed. Use Install / repair first.' }
        if ([string]$taskService.Status -eq $State) { return $taskService }
        if ($State -eq 'Running' -and [string]$taskService.Status -eq 'Stopped') { throw 'The service exited during startup. Check the service log in Settings.' }
        if ($taskTimer.Elapsed.TotalSeconds -ge 30) { throw "Timed out waiting for the service to become $State (currently $($taskService.Status))." }
        Start-Sleep -Milliseconds 200
    } while ($true)
}

function Start-ManagedService {
    $taskService = Get-ManagedService
    if (-not $taskService) { throw 'The service is not installed. Use Install / repair first.' }
    if ([string]$taskService.Status -eq 'StopPending') { $taskService = Wait-ManagedService 'Stopped' }
    if ([string]$taskService.Status -eq 'StartPending') { Wait-ManagedService 'Running' | Out-Null; return }
    if ([string]$taskService.Status -eq 'Running') { return }
    if ([string]$taskService.StartType -eq 'Disabled') { throw 'The service is disabled in Windows. Use Install / repair to enable automatic startup.' }
    Start-Service -Name SpeedLimitFree -ErrorAction Stop
    Wait-ManagedService 'Running' | Out-Null
    Start-Sleep -Milliseconds 700
    if ([string](Get-ManagedService).Status -ne 'Running') { throw 'The service exited during startup. Check the service log in Settings.' }
}

function Stop-ManagedService {
    $taskService = Get-ManagedService
    if (-not $taskService -or [string]$taskService.Status -eq 'Stopped') { return }
    if ([string]$taskService.Status -eq 'StartPending') { $taskService = Wait-ManagedService 'Running' }
    $taskRegistration = Get-ServiceRegistration
    $taskProcess = if ($taskRegistration.ProcessId) { Get-Process -Id $taskRegistration.ProcessId -ErrorAction SilentlyContinue }
    if ([string]$taskService.Status -ne 'StopPending') { Stop-Service -Name SpeedLimitFree -ErrorAction Stop -NoWait }
    Wait-ManagedService 'Stopped' | Out-Null
    if ($taskProcess -and -not $taskProcess.WaitForExit(20000)) { throw 'The service stopped but its process has not exited. Files were not replaced.' }
}

function Get-ServiceRegistration { Get-CimInstance Win32_Service -Filter "Name='SpeedLimitFree'" }

function Set-ServiceRegistration([string]$Binary, [string]$StartMode = 'Automatic') {
    $taskRegistration = Get-ServiceRegistration
    if ($taskRegistration) {
        $taskChange = Invoke-CimMethod -InputObject $taskRegistration -MethodName Change -Arguments @{ PathName = $Binary; StartMode = $StartMode }
        if ($taskChange.ReturnValue -ne 0) { throw "Could not update service registration (Windows code $($taskChange.ReturnValue))." }
    } else { New-Service -Name SpeedLimitFree -BinaryPathName $Binary -DisplayName 'SpeedLimitFree Traffic Service' -Description 'Per-application network bandwidth limits.' -StartupType $StartMode | Out-Null }
}

function Set-ProtectedDirectory([string]$Path, [Security.Principal.SecurityIdentifier]$Owner) {
    if (Test-Path -LiteralPath $Path) {
        if ((Get-Item -LiteralPath $Path -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw "Setup directory must not be a link: $Path" }
    } else { New-Item -ItemType Directory -Path $Path -Force | Out-Null }
    $taskAcl = New-Object Security.AccessControl.DirectorySecurity
    $taskAcl.SetAccessRuleProtection($true, $false)
    foreach ($taskSID in @('S-1-5-18', 'S-1-5-32-544')) {
        $taskPrincipal = New-Object Security.Principal.SecurityIdentifier($taskSID)
        $taskAcl.AddAccessRule((New-Object Security.AccessControl.FileSystemAccessRule($taskPrincipal, 'FullControl', 'ContainerInherit,ObjectInherit', 'None', 'Allow')))
    }
    $taskAcl.AddAccessRule((New-Object Security.AccessControl.FileSystemAccessRule($Owner, 'ReadAndExecute', 'ContainerInherit,ObjectInherit', 'None', 'Allow')))
    Set-Acl -LiteralPath $Path -AclObject $taskAcl
}

function Test-SameFile([string]$Left, [string]$Right) {
    if (-not (Test-Path -LiteralPath $Right -PathType Leaf)) { return $false }
    return (Get-FileHash -LiteralPath $Left -Algorithm SHA256).Hash -eq (Get-FileHash -LiteralPath $Right -Algorithm SHA256).Hash
}

function Remove-SetupDirectory([string]$Path, [string]$Install) {
    $taskResolved = [IO.Path]::GetFullPath($Path)
    $taskParent = [IO.Path]::GetFullPath($Install).TrimEnd('\') + '\'
    if (-not $taskResolved.StartsWith($taskParent, [StringComparison]::OrdinalIgnoreCase) -or [IO.Path]::GetFileName($taskResolved) -notlike '.setup-*') { throw 'Refusing cleanup outside the setup staging directory.' }
    if (Test-Path -LiteralPath $taskResolved) { Remove-Item -LiteralPath $taskResolved -Recurse -Force }
}

function Invoke-ServiceSetup([string]$Source, [string]$Install, [string]$Data, [Security.Principal.SecurityIdentifier]$Owner, [string[]]$RequiredFiles = @(), [scriptblock]$Finalize) {
    # Stage payload and backups before interrupting an existing service.
    $taskCore = @('SpeedLimitFree-service.exe', 'WinDivert.dll', 'WinDivert64.sys')
    $taskCore = @($taskCore + $RequiredFiles | Select-Object -Unique)
    foreach ($taskFile in ($taskCore + 'SpeedLimitFree.exe')) {
        if (-not (Test-Path -LiteralPath (Join-Path $Source $taskFile) -PathType Leaf)) { throw "Missing $taskFile beside this installer. Extract the complete release folder first." }
    }
    Set-ProtectedDirectory $Install $Owner
    Set-ProtectedDirectory $Data $Owner
    $taskStage = Join-Path $Install ('.setup-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $taskStage | Out-Null
    $taskBackup = Join-Path $taskStage 'previous'
    New-Item -ItemType Directory -Path $taskBackup | Out-Null
    $taskKeepBackup = $false
    try {
        $taskChanged = @()
        foreach ($taskFile in $taskCore) {
            $taskFrom = Join-Path $Source $taskFile
            $taskTo = Join-Path $Install $taskFile
            if (-not (Test-SameFile $taskFrom $taskTo)) {
                Copy-Item -LiteralPath $taskFrom -Destination (Join-Path $taskStage $taskFile)
                if (Test-Path -LiteralPath $taskTo) { Copy-Item -LiteralPath $taskTo -Destination (Join-Path $taskBackup $taskFile) }
                $taskChanged += $taskFile
            }
        }
        $taskPrevious = Get-ServiceRegistration
        $taskWasRunning = $taskPrevious -and $taskPrevious.State -in @('Running', 'Start Pending')
        $taskBinary = '"' + (Join-Path $Install 'SpeedLimitFree-service.exe') + '" --service --owner ' + $Owner.Value + ' --data "' + $Data + '" --driver "' + $Install + '"'
        $taskConfigChanged = -not $taskPrevious -or $taskPrevious.PathName -ne $taskBinary -or $taskPrevious.StartMode -ne 'Auto'
        $taskApplied = @()
        $taskRegistrationTouched = $false
        $taskInterrupted = $false
        try {
            if ($taskChanged.Count -or $taskConfigChanged) {
                $taskInterrupted = $true
                Stop-ManagedService
                foreach ($taskFile in $taskChanged) {
                    $taskApplied += $taskFile
                    Copy-Item -LiteralPath (Join-Path $taskStage $taskFile) -Destination (Join-Path $Install $taskFile) -Force
                }
                if ($taskConfigChanged) { $taskRegistrationTouched = $true; Set-ServiceRegistration $taskBinary }
            }
            Start-ManagedService
            if ($RequiredFiles.Count) { Set-ServiceExtras $Install }
            if ($Finalize) { & $Finalize }
        } catch {
            $taskFailure = $_.Exception.Message
            if ($taskInterrupted) {
                try {
                    Stop-ManagedService
                    foreach ($taskFile in $taskApplied) {
                        $taskOld = Join-Path $taskBackup $taskFile
                        if (Test-Path -LiteralPath $taskOld) {
                            if (-not (Test-SameFile $taskOld (Join-Path $Install $taskFile))) { Copy-Item -LiteralPath $taskOld -Destination (Join-Path $Install $taskFile) -Force }
                        }
                        else { Remove-Item -LiteralPath (Join-Path $Install $taskFile) -Force -ErrorAction SilentlyContinue }
                    }
                    if ($taskRegistrationTouched) {
                        if ($taskPrevious) {
                            $taskOldMode = if ($taskPrevious.StartMode -eq 'Auto') { 'Automatic' } else { $taskPrevious.StartMode }
                            Set-ServiceRegistration $taskPrevious.PathName $taskOldMode
                        } else {
                            & sc.exe delete SpeedLimitFree | Out-Null
                            if ($LASTEXITCODE) { throw 'Could not remove failed service registration.' }
                        }
                    }
                    if ($taskWasRunning) { Start-ManagedService }
                    $taskFailure += ' Previous service files and configuration were restored.'
                } catch {
                    $taskKeepBackup = $true
                    $taskFailure += " Recovery also failed: $($_.Exception.Message) Backup retained at $taskBackup."
                }
            }
            throw $taskFailure
        }
        # A running desktop can lock its executable; this must never undo service setup.
        $taskWarnings = @()
        foreach ($taskFile in @('SpeedLimitFree.exe', 'limiter-debug.exe', 'install-service.ps1', 'manage-service.ps1', 'service-common.ps1', 'uninstall-service.ps1', 'installer-common.ps1', 'installer-actions.ps1', 'installer-native.cs', 'START_HERE.md', 'THIRD_PARTY_NOTICES.md', 'LICENSE', 'COPYING', 'COPYING.LESSER', 'distribution.json')) {
            $taskFrom = Join-Path $Source $taskFile
            $taskTo = Join-Path $Install $taskFile
            if (Test-Path -LiteralPath $taskFrom) {
                try { if (-not (Test-SameFile $taskFrom $taskTo)) { Copy-Item -LiteralPath $taskFrom -Destination $taskTo -Force } }
                catch { $taskWarnings += "Could not update ${taskFile}: $($_.Exception.Message)" }
            }
        }
        try { Set-ServiceExtras $Install } catch { $taskWarnings += $_.Exception.Message }
        $taskMessage = 'Service is installed and running. Saved rules were retained.'
        if ($taskWarnings.Count) { $taskMessage += ' Some desktop or support files could not be updated. Quit the app from the tray and rerun setup from the new release folder. ' + ($taskWarnings -join ' ') }
        return $taskMessage
    } finally { if (-not $taskKeepBackup) { Remove-SetupDirectory $taskStage $Install } }
}

function Set-ServiceExtras([string]$Install) {
    & sc.exe failure SpeedLimitFree reset= 86400 actions= restart/5000/restart/15000/restart/60000 | Out-Null
    if ($LASTEXITCODE) { throw 'Could not configure automatic service recovery.' }
    $taskShell = New-Object -ComObject WScript.Shell
    $taskShortcut = $taskShell.CreateShortcut((Join-Path ([Environment]::GetFolderPath('CommonPrograms')) 'SpeedLimitFree.lnk'))
    $taskShortcut.TargetPath = Join-Path $Install 'SpeedLimitFree.exe'
    $taskShortcut.WorkingDirectory = $Install
    $taskShortcut.Save()
}
