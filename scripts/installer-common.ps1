$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'service-common.ps1')
$script:UninstallKey = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\SpeedLimitFree'
$script:PayloadFiles = @('SpeedLimitFree.exe','SpeedLimitFree-service.exe','limiter-debug.exe','WinDivert.dll','WinDivert64.sys','install-service.ps1','manage-service.ps1','service-common.ps1','uninstall-service.ps1','installer-common.ps1','installer-actions.ps1','installer-native.cs','Uninstall.exe','START_HERE.md','THIRD_PARTY_NOTICES.md','LICENSE','distribution.json')

function Get-InstallerLayout {
    [pscustomobject]@{
        Install = Join-Path $env:ProgramFiles 'SpeedLimitFree'
        Data = Join-Path $env:ProgramData 'SpeedLimitFree'
        Programs = [Environment]::GetFolderPath('CommonPrograms')
        Desktop = [Environment]::GetFolderPath('CommonDesktopDirectory')
        Profiles = @(Get-CimInstance Win32_UserProfile | Where-Object { -not $_.Special -and $_.LocalPath } | ForEach-Object {
            [pscustomobject]@{ SID = $_.SID; Path = $_.LocalPath }
        })
    }
}

function Assert-ChildPath([string]$Path, [string]$Parent) {
    $taskFull = [IO.Path]::GetFullPath($Path).TrimEnd('\')
    $taskParent = [IO.Path]::GetFullPath($Parent).TrimEnd('\')
    if (-not $taskFull.StartsWith($taskParent + '\', [StringComparison]::OrdinalIgnoreCase)) { throw "Refusing cleanup outside $taskParent`: $taskFull" }
    # Never traverse a junction/symlink in the ancestors of a cleanup root.
    $taskAncestor = [IO.Directory]::GetParent($taskFull)
    while ($taskAncestor) {
        if ($taskAncestor.Exists -and ($taskAncestor.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw "Cleanup path crosses a link: $($taskAncestor.FullName)" }
        $taskAncestor = $taskAncestor.Parent
    }
    return $taskFull
}

function Remove-OwnedTree([string]$Path, [string]$Parent) {
    $taskFull = Assert-ChildPath $Path $Parent
    if (-not (Test-Path -LiteralPath $taskFull)) { return }
    $taskItem = Get-Item -LiteralPath $taskFull -Force
    if ($taskItem.Attributes -band [IO.FileAttributes]::ReparsePoint) {
        # Delete only the link itself. Never enumerate its target.
        if ($taskItem.PSIsContainer) { [IO.Directory]::Delete($taskFull) }
        else { Remove-Item -LiteralPath $taskFull -Force }
        return
    }
    if ($taskItem.PSIsContainer) {
        foreach ($taskChild in @(Get-ChildItem -LiteralPath $taskFull -Force | Sort-Object @{Expression={ if ($_.Name -eq 'Uninstall.exe') { 1 } else { 0 } }})) { Remove-OwnedTree $taskChild.FullName $taskFull }
        Remove-Item -LiteralPath $taskFull -Force
    } else { Remove-Item -LiteralPath $taskFull -Force }
}

function Initialize-InstallerNative {
    if (-not ('SpeedLimitFreeInstaller.Native' -as [type])) { Add-Type -Path (Join-Path $PSScriptRoot 'installer-native.cs') }
}

function Get-InstallOwner($Layout) {
    # An upgrade keeps the existing IPC owner, including over-the-shoulder UAC.
    $taskRegistration = Get-ServiceRegistration
    if ($taskRegistration -and $taskRegistration.PathName -match '--owner\s+(S-1-[0-9-]+)') { return New-Object Security.Principal.SecurityIdentifier($Matches[1]) }
    Initialize-InstallerNative
    $taskExplorerPID = [SpeedLimitFreeInstaller.Native]::ShellProcess()
    if (-not $taskExplorerPID) { throw 'Start setup from the Windows desktop so it can identify the account that will use the application.' }
    $taskExplorer = Get-CimInstance Win32_Process -Filter "ProcessId=$taskExplorerPID"
    $taskOwner = Invoke-CimMethod -InputObject $taskExplorer -MethodName GetOwnerSid
    if ($taskOwner.ReturnValue -ne 0 -or -not $taskOwner.Sid) { throw 'Could not identify the Windows desktop account.' }
    return New-Object Security.Principal.SecurityIdentifier($taskOwner.Sid)
}

function Close-InstalledDesktop([string]$Install) {
    Initialize-InstallerNative
    $taskTarget = Join-Path $Install 'SpeedLimitFree.exe'
    $taskProcesses = @(Get-CimInstance Win32_Process -Filter "Name LIKE 'SpeedLimitFree%.exe'" | Where-Object { $_.Name -match '^SpeedLimitFree(?:-\d+\.\d+\.\d+)?\.exe$' })
    foreach ($taskProcess in $taskProcesses) {
        $taskHandle = Get-Process -Id $taskProcess.ProcessId -ErrorAction SilentlyContinue
        if (-not $taskHandle) { continue }
        $taskSent = [SpeedLimitFreeInstaller.Native]::QuitDesktop($taskProcess.ProcessId)
        # A matching tray class identifies older ZIP desktops too. Never terminate
        # an unrelated executable just because it happens to have the same name.
        if (-not $taskSent -and -not [string]::Equals($taskProcess.ExecutablePath, $taskTarget, [StringComparison]::OrdinalIgnoreCase)) { continue }
        if (-not $taskHandle.WaitForExit(12000)) { throw 'Quit SpeedLimitFree from its tray in every signed-in session, then retry. The service and saved rules have not been removed.' }
    }
}

function Get-ProfileFolders($Profile) {
    $taskBase = $Profile.Path
    $taskLocal = Join-Path $taskBase 'AppData\Local'
    $taskRoaming = Join-Path $taskBase 'AppData\Roaming'
    # Honor redirected app-data folders for loaded profiles, without loading hives.
    $taskUsers = [Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]::Users, [Microsoft.Win32.RegistryView]::Registry64)
    try {
        $taskKey = $taskUsers.OpenSubKey($Profile.SID + '\Software\Microsoft\Windows\CurrentVersion\Explorer\User Shell Folders')
        if ($taskKey) {
            try {
                # Read raw expandable values: another account may have accepted UAC.
                $taskOptions = [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames
                $taskLocalValue = $taskKey.GetValue('Local AppData', $null, $taskOptions)
                $taskRoamingValue = $taskKey.GetValue('AppData', $null, $taskOptions)
                if ($taskLocalValue) { $taskLocal = ([string]$taskLocalValue).Replace('%USERPROFILE%', $taskBase) }
                if ($taskRoamingValue) { $taskRoaming = ([string]$taskRoamingValue).Replace('%USERPROFILE%', $taskBase) }
            } finally { $taskKey.Dispose() }
        }
    } finally { $taskUsers.Dispose() }
    if ($taskLocal.Contains('%') -or $taskRoaming.Contains('%')) { throw "Unsupported app-data folder expansion for profile $($Profile.SID)." }
    # A redirect outside the user's profile needs deliberate support, never broad deletion.
    Assert-ChildPath (Join-Path $taskLocal 'SpeedLimitFree') $taskBase | Out-Null
    Assert-ChildPath (Join-Path $taskRoaming 'SpeedLimitFree.exe') $taskBase | Out-Null
    [pscustomobject]@{ Local = $taskLocal; Roaming = $taskRoaming; Startup = Join-Path $taskRoaming 'Microsoft\Windows\Start Menu\Programs\Startup' }
}

function Set-TrayStartup($Layout, [string]$SID, [bool]$Enabled) {
    $taskProfile = @($Layout.Profiles | Where-Object SID -eq $SID)[0]
    if (-not $taskProfile) { throw 'The application owner profile could not be found.' }
    $taskFolders = Get-ProfileFolders $taskProfile
    $taskShortcut = Join-Path $taskFolders.Startup 'SpeedLimitFree.lnk'
    Assert-ChildPath $taskShortcut $taskProfile.Path | Out-Null
    if ($Enabled) {
        New-Item -ItemType Directory -Path $taskFolders.Startup -Force | Out-Null
        $taskShell = New-Object -ComObject WScript.Shell
        $taskLink = $taskShell.CreateShortcut($taskShortcut)
        $taskLink.TargetPath = Join-Path $Layout.Install 'SpeedLimitFree.exe'
        $taskLink.Arguments = '--tray'
        $taskLink.WorkingDirectory = $Layout.Install
        $taskLink.Save()
    } elseif (Test-Path -LiteralPath $taskShortcut) { Remove-Item -LiteralPath $taskShortcut -Force }
}

function Set-InstalledRegistration($Layout, [string]$Version, [string]$SID, [bool]$Startup) {
    New-Item -Path $script:UninstallKey -Force | Out-Null
    $taskValues = @{
        DisplayName = 'SpeedLimitFree'; DisplayVersion = $Version; Publisher = 'gusdeyw'
        URLInfoAbout = 'https://github.com/gusdeyw'
        InstallLocation = $Layout.Install; DisplayIcon = (Join-Path $Layout.Install 'SpeedLimitFree.exe')
        UninstallString = ('"' + (Join-Path $Layout.Install 'Uninstall.exe') + '"')
        QuietUninstallString = ('"' + (Join-Path $Layout.Install 'Uninstall.exe') + '" /S')
        OwnerSID = $SID
    }
    foreach ($taskName in $taskValues.Keys) { New-ItemProperty -LiteralPath $script:UninstallKey -Name $taskName -Value $taskValues[$taskName] -PropertyType String -Force | Out-Null }
    foreach ($taskName in @('NoModify','NoRepair')) { New-ItemProperty -LiteralPath $script:UninstallKey -Name $taskName -Value 1 -PropertyType DWord -Force | Out-Null }
    New-ItemProperty -LiteralPath $script:UninstallKey -Name 'TrayStartup' -Value ([int]$Startup) -PropertyType DWord -Force | Out-Null
    $taskSize = [int][Math]::Ceiling((Get-ChildItem -LiteralPath $Layout.Install -File | Measure-Object Length -Sum).Sum / 1024)
    New-ItemProperty -LiteralPath $script:UninstallKey -Name 'EstimatedSize' -Value $taskSize -PropertyType DWord -Force | Out-Null
}

function Restore-InstalledRegistration($Previous) {
    if (Test-Path -LiteralPath $script:UninstallKey) { Remove-Item -LiteralPath $script:UninstallKey -Recurse -Force }
    if ($Previous) {
        New-Item -Path $script:UninstallKey -Force | Out-Null
        foreach ($taskProperty in $Previous.PSObject.Properties | Where-Object Name -NotLike 'PS*') {
            $taskType = if ($taskProperty.Value -is [int]) { 'DWord' } else { 'String' }
            New-ItemProperty -LiteralPath $script:UninstallKey -Name $taskProperty.Name -Value $taskProperty.Value -PropertyType $taskType -Force | Out-Null
        }
    }
}

function Install-DesktopPackage($Layout, [string]$Source, [string]$Version, [bool]$Startup) {
    foreach ($taskFile in $script:PayloadFiles) { if (-not (Test-Path -LiteralPath (Join-Path $Source $taskFile) -PathType Leaf)) { throw "Incomplete installer: missing $taskFile" } }
    $taskOwner = Get-InstallOwner $Layout
    $taskPrevious = Get-ItemProperty -LiteralPath $script:UninstallKey -ErrorAction SilentlyContinue
    if ($taskPrevious -and [version]$taskPrevious.DisplayVersion -gt [version]$Version) { throw 'A newer version is installed. Use the same or a newer installer.' }
    $taskHadInstall = Test-Path -LiteralPath $Layout.Install
    $taskHadData = Test-Path -LiteralPath $Layout.Data
    Close-InstalledDesktop $Layout.Install
    try {
        $taskFinalize = {
            Set-TrayStartup $Layout $taskOwner.Value $Startup
            Set-InstalledRegistration $Layout $Version $taskOwner.Value $Startup
        }
        Invoke-ServiceSetup $Source $Layout.Install $Layout.Data $taskOwner -RequiredFiles $script:PayloadFiles -Finalize $taskFinalize | Out-Null
    } catch {
        $taskFailure = $_.Exception.Message
        try { Restore-InstalledRegistration $taskPrevious; Set-TrayStartup $Layout $taskOwner.Value ([bool]($taskPrevious -and $taskPrevious.TrayStartup)) }
        catch { $taskFailure += " Registration recovery failed: $($_.Exception.Message)" }
        if (-not (Get-ManagedService)) {
            if (-not $taskHadInstall) { Remove-OwnedTree $Layout.Install ([IO.Path]::GetDirectoryName($Layout.Install)) }
            if (-not $taskHadData) { Remove-OwnedTree $Layout.Data ([IO.Path]::GetDirectoryName($Layout.Data)) }
            $taskLink = Join-Path $Layout.Programs 'SpeedLimitFree.lnk'
            if (-not $taskHadInstall -and (Test-Path -LiteralPath $taskLink)) { Remove-Item -LiteralPath $taskLink -Force }
        }
        throw $taskFailure
    }
}

function Get-DriverClients([string]$DLL) {
    Initialize-InstallerNative
    return [SpeedLimitFreeInstaller.Native]::DriverClients($DLL)
}

function Remove-OwnedDriver([string]$Install) {
    $taskExpected = Join-Path $Install 'WinDivert64.sys'
    foreach ($taskDriver in @(Get-CimInstance Win32_SystemDriver -Filter "Name LIKE 'WinDivert%'")) {
        $taskPath = ([string]$taskDriver.PathName).Trim('"') -replace '^\\\?\?\\', ''
        if (-not [string]::Equals($taskPath, $taskExpected, [StringComparison]::OrdinalIgnoreCase)) { continue }
        if ($taskDriver.State -ne 'Stopped') {
            $taskClients = @(Get-DriverClients (Join-Path $Install 'WinDivert.dll'))
            if ($taskClients.Count) { throw "Another application is using this copy of WinDivert (process IDs: $($taskClients -join ', ')). Close it and retry uninstall. Shared traffic interception was left running." }
            & sc.exe stop $taskDriver.Name | Out-Null
            if ($LASTEXITCODE -notin @(0,1062)) { throw 'Windows could not unload this application''s WinDivert driver. Restart Windows and retry uninstall.' }
            $taskTimer = [Diagnostics.Stopwatch]::StartNew()
            do {
                $taskCurrent = @(Get-CimInstance Win32_SystemDriver -Filter "Name LIKE 'WinDivert%'" | Where-Object Name -eq $taskDriver.Name)
                if (-not $taskCurrent.Count -or $taskCurrent[0].State -eq 'Stopped') { break }
                if ($taskTimer.Elapsed.TotalSeconds -gt 20) { throw 'The driver is still stopping. Restart Windows and retry uninstall.' }
                Start-Sleep -Milliseconds 200
            } while ($true)
        }
        & sc.exe delete $taskDriver.Name | Out-Null
        if ($LASTEXITCODE -notin @(0,1060,1072)) { throw 'Could not unregister this application''s WinDivert driver.' }
    }
}

function Remove-DesktopPackage($Layout) {
    # Resolve every removal root before stopping anything. No user-supplied install path.
    $taskTargets = @(
        [pscustomobject]@{ Path = $Layout.Data; Parent = [IO.Path]::GetDirectoryName($Layout.Data) }
        [pscustomobject]@{ Path = $Layout.Install; Parent = [IO.Path]::GetDirectoryName($Layout.Install) }
    )
    $taskLinks = @((Join-Path $Layout.Programs 'SpeedLimitFree.lnk'), (Join-Path $Layout.Desktop 'SpeedLimitFree.lnk'))
    foreach ($taskProfile in $Layout.Profiles) {
        $taskFolders = Get-ProfileFolders $taskProfile
        $taskTargets += [pscustomobject]@{ Path = (Join-Path $taskFolders.Local 'SpeedLimitFree'); Parent = $taskProfile.Path }
        # Wails <=0.2.0 used AppData\Roaming\<executable name>.
        if (Test-Path -LiteralPath $taskFolders.Roaming) {
            foreach ($taskOld in Get-ChildItem -LiteralPath $taskFolders.Roaming -Directory -Force | Where-Object { $_.Name -match '^SpeedLimitFree(?:-\d+\.\d+\.\d+)?\.exe$' }) {
                $taskTargets += [pscustomobject]@{ Path = $taskOld.FullName; Parent = $taskProfile.Path }
            }
        }
        $taskLinks += Join-Path $taskFolders.Startup 'SpeedLimitFree.lnk'
    }
    $taskTargets = @($taskTargets | Sort-Object @{Expression={ if ($_.Path -eq $Layout.Install) { 1 } else { 0 } }})
    foreach ($taskTarget in $taskTargets) { Assert-ChildPath $taskTarget.Path $taskTarget.Parent | Out-Null }
    foreach ($taskLink in $taskLinks) { Assert-ChildPath $taskLink ([IO.Path]::GetDirectoryName($taskLink)) | Out-Null }
    $taskUninstaller = Join-Path $Layout.Install 'Uninstall.exe'
    $taskRecovery = $null
    if (Test-Path -LiteralPath $taskUninstaller) { $taskRecovery = [IO.File]::ReadAllBytes($taskUninstaller) }
    try {
    Close-InstalledDesktop $Layout.Install
    Stop-ManagedService
    Remove-OwnedDriver $Layout.Install
    if (Get-ManagedService) {
        & sc.exe delete SpeedLimitFree | Out-Null
        if ($LASTEXITCODE -notin @(0,1060,1072)) { throw 'Could not unregister the SpeedLimitFree service.' }
        $taskTimer = [Diagnostics.Stopwatch]::StartNew()
        while (Get-ManagedService) {
            if ($taskTimer.Elapsed.TotalSeconds -gt 15) { throw 'Windows still holds the service registration. Close Services and retry uninstall.' }
            Start-Sleep -Milliseconds 200
        }
    }
    foreach ($taskLink in $taskLinks) { if (Test-Path -LiteralPath $taskLink) { Remove-Item -LiteralPath $taskLink -Force } }
    # Keep Installed Apps registration until all file cleanup succeeds, so a failed
    # uninstall remains discoverable and can be retried. Never claim partial success.
    foreach ($taskTarget in $taskTargets) {
        $taskAttempts = 0
        while ($true) {
            try { Remove-OwnedTree $taskTarget.Path $taskTarget.Parent; break }
            catch { if (++$taskAttempts -ge 8) { throw }; Start-Sleep -Milliseconds 500 }
        }
    }
    if (Test-Path -LiteralPath $script:UninstallKey) { Remove-Item -LiteralPath $script:UninstallKey -Recurse -Force }
    } catch {
        # A late failure must still leave a callable embedded uninstaller for retry.
        if ($taskRecovery -and -not (Test-Path -LiteralPath $taskUninstaller)) {
            Assert-ChildPath $taskUninstaller ([IO.Path]::GetDirectoryName($Layout.Install)) | Out-Null
            New-Item -ItemType Directory -Path $Layout.Install -Force | Out-Null
            [IO.File]::WriteAllBytes($taskUninstaller, $taskRecovery)
        }
        throw
    }
}
