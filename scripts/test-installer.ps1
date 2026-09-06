# Real file transactions, shortcuts, user-registry metadata, cleanup, junctions,
# and locks. Service/driver operations and installation roots are isolated fakes.
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'installer-common.ps1')
$taskTools = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\.tools'))
$taskTestRoot = Join-Path $taskTools ('installer-tests-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $taskTestRoot -Force | Out-Null
$script:UninstallKey = 'HKCU:\Software\SpeedLimitFree.InstallerTests.' + [guid]::NewGuid().ToString('N')
$taskOwner = New-Object Security.Principal.SecurityIdentifier('S-1-5-21-1234-1234-1234-1001')
$script:checks = 0; $script:cases = 0
$taskRegistrationFunction = (Get-Command Set-InstalledRegistration).ScriptBlock
function Assert([bool]$Condition, [string]$Message) { if (-not $Condition) { throw "ASSERT: $Message" }; $script:checks++ }
function Fail([scriptblock]$Action, [string]$Text) { $taskError = ''; try { & $Action | Out-Null } catch { $taskError = $_.Exception.Message }; Assert ($taskError.Contains($Text)) "Expected '$Text'; got '$taskError'" }
function Set-ProtectedDirectory([string]$Path, $Owner) { New-Item -ItemType Directory -Path $Path -Force | Out-Null }
function Get-InstallOwner($Layout) { $taskOwner }
function Close-InstalledDesktop([string]$Install) { if ($script:desktopBusy) { throw 'Quit SpeedLimitFree from its tray' } }
function Get-ManagedService { if ($script:registered) { [pscustomobject]@{ Status=$script:state; StartType='Automatic' } } }
function Get-ServiceRegistration { if ($script:registered) { [pscustomobject]@{ State=$script:state; StartMode='Auto'; PathName=$script:binary; ProcessId=0 } } }
function Set-ServiceRegistration([string]$Binary, [string]$StartMode) { $script:registered=$true; $script:binary=$Binary }
function Stop-Service { $script:stops++; $script:state='Stopped' }
function Start-Service { $script:starts++; if ($script:failStart -and [IO.File]::ReadAllText((Join-Path $script:layout.Install 'SpeedLimitFree-service.exe')) -eq 'new') { throw 'Simulated service failure' }; $script:state='Running' }
function Set-ServiceExtras([string]$Install) { [IO.File]::WriteAllText((Join-Path $script:layout.Programs 'SpeedLimitFree.lnk'), 'shortcut') }
function Get-CimInstance { param([string]$ClassName,[string]$Filter); if ($ClassName -eq 'Win32_SystemDriver') { return $script:drivers }; throw 'Unexpected real CIM query in fixture' }
function Get-DriverClients([string]$DLL) { $script:clients }
function sc.exe { param($Action,$Name); $global:LASTEXITCODE=0; $script:scCalls += "$Action $Name"; if ($Action -eq 'delete' -and $Name -eq 'SpeedLimitFree') { $script:registered=$false }; if ($Action -eq 'stop') { foreach ($taskDriver in $script:drivers | Where-Object Name -eq $Name) { $taskDriver.State='Stopped' } } }
function Set-InstalledRegistration($Layout, [string]$Version, [string]$SID, [bool]$Startup) { & $taskRegistrationFunction $Layout $Version $SID $Startup; if ($script:failFinalize) { throw 'Simulated registration failure' } }
function Reset-Fixture {
    if (Test-Path -LiteralPath $script:UninstallKey) { Remove-Item -LiteralPath $script:UninstallKey -Recurse -Force }
    $script:cases++
    $taskCase = Join-Path $taskTestRoot ([string]$script:cases)
    $script:layout = [pscustomobject]@{
        Install=Join-Path $taskCase 'installed\SpeedLimitFree'; Data=Join-Path $taskCase 'data\SpeedLimitFree'
        Programs=Join-Path $taskCase 'programs'; Desktop=Join-Path $taskCase 'desktop'
        Profiles=@([pscustomobject]@{SID=$taskOwner.Value; Path=Join-Path $taskCase "User ' One"},[pscustomobject]@{SID='S-1-5-21-1234-1234-1234-1002'; Path=Join-Path $taskCase 'User Two'})
    }
    $script:source = Join-Path $taskCase 'source'
    foreach ($taskDir in @($script:source,$script:layout.Programs,$script:layout.Desktop)) { New-Item -ItemType Directory -Path $taskDir -Force | Out-Null }
    foreach ($taskFile in $script:PayloadFiles) { [IO.File]::WriteAllText((Join-Path $script:source $taskFile), 'old') }
    $script:registered=$false; $script:state='Stopped'; $script:binary=''; $script:starts=0; $script:stops=0
    $script:failStart=$false; $script:failFinalize=$false; $script:desktopBusy=$false; $script:drivers=@(); $script:clients=@(); $script:scCalls=@()
}
function Install([string]$Version='0.3.0', [bool]$Startup=$true) { Install-DesktopPackage $script:layout $script:source $Version $Startup }
function Write-File([string]$Path, [string]$Value='test') { New-Item -ItemType Directory -Path ([IO.Path]::GetDirectoryName($Path)) -Force | Out-Null; [IO.File]::WriteAllText($Path,$Value) }
try {
    Reset-Fixture
    Install
    Assert ($script:registered -and $script:state -eq 'Running') 'Fresh install starts registered service'
    Assert ((Get-ItemProperty -LiteralPath $script:UninstallKey).DisplayVersion -eq '0.3.0') 'Installed Apps metadata is written'
    $taskStartup = Join-Path (Get-ProfileFolders $script:layout.Profiles[0]).Startup 'SpeedLimitFree.lnk'
    $taskShell = New-Object -ComObject WScript.Shell
    Assert ($taskShell.CreateShortcut($taskStartup).Arguments -eq '--tray') 'Optional startup uses hidden tray mode'
    Write-File (Join-Path $script:layout.Data 'rules.json') 'keep rules on update'
    Install
    Assert ($script:stops -eq 0 -and $script:starts -eq 1) 'Unchanged repair does not restart service'
    Write-File (Join-Path $script:source 'SpeedLimitFree.exe') 'new desktop'
    Install '0.3.1' $false
    Assert ([IO.File]::ReadAllText((Join-Path $script:layout.Install 'SpeedLimitFree.exe')) -eq 'new desktop') 'Upgrade replaces desktop'
    Assert ([IO.File]::ReadAllText((Join-Path $script:layout.Data 'rules.json')) -eq 'keep rules on update') 'Upgrade preserves rules'
    Assert (-not (Test-Path -LiteralPath $taskStartup)) 'Startup choice can be disabled'
    Fail { Install '0.3.0' } 'newer version'

    Reset-Fixture
    Write-File (Join-Path $script:source 'SpeedLimitFree-service.exe') 'new'
    $script:failStart=$true
    Fail { Install } 'Simulated service failure'
    Assert (-not $script:registered -and -not (Test-Path -LiteralPath $script:layout.Install)) 'Failed fresh install removes new service and payload'
    Assert (-not (Test-Path -LiteralPath $script:layout.Data) -and -not (Test-Path -LiteralPath $script:UninstallKey)) 'Failed fresh install leaves no app data or Installed Apps entry'

    Reset-Fixture; Install
    Write-File (Join-Path $script:source 'SpeedLimitFree-service.exe') 'new'
    Write-File (Join-Path $script:source 'Uninstall.exe') 'new uninstaller'
    $script:failStart=$true
    Fail { Install '0.3.1' } 'Previous service files and configuration were restored'
    Assert ([IO.File]::ReadAllText((Join-Path $script:layout.Install 'Uninstall.exe')) -eq 'old') 'Failed startup restores previous uninstaller too'
    Assert ($script:state -eq 'Running') 'Failed update restarts previous service'
    Assert ((Get-ItemProperty -LiteralPath $script:UninstallKey).DisplayVersion -eq '0.3.0') 'Failed update keeps old Installed Apps version'

    Reset-Fixture; Install
    Write-File (Join-Path $script:source 'SpeedLimitFree.exe') 'new desktop'
    $script:failFinalize=$true
    Fail { Install '0.3.1' $false } 'Simulated registration failure'
    Assert ([IO.File]::ReadAllText((Join-Path $script:layout.Install 'SpeedLimitFree.exe')) -eq 'old') 'Metadata failure rolls files back'
    Assert ((Get-ItemProperty -LiteralPath $script:UninstallKey).TrayStartup -eq 1) 'Metadata failure restores startup selection'

    Reset-Fixture; Install
    $script:desktopBusy=$true
    Fail { Remove-DesktopPackage $script:layout } 'Quit SpeedLimitFree'
    Assert ($script:stops -eq 0) 'Busy desktop blocks uninstall before service changes'
    $script:desktopBusy=$false
    $script:drivers=@([pscustomobject]@{Name='WinDivert'; State='Running'; PathName=(Join-Path $script:layout.Install 'WinDivert64.sys')})
    $script:clients=@(456)
    Fail { Remove-DesktopPackage $script:layout } 'Another application is using'
    Assert ($script:registered -and (Test-Path -LiteralPath $script:layout.Install)) 'Shared driver prevents partial file removal'
    Assert ($script:scCalls.Count -eq 0) 'Shared active driver is not stopped or deleted'
    $script:drivers=@([pscustomobject]@{Name='WinDivert'; State='Running'; PathName='C:\OtherApplication\WinDivert64.sys'})
    Remove-OwnedDriver $script:layout.Install
    Assert ($script:scCalls.Count -eq 0) 'Driver owned by another installation is untouched'
    $script:drivers=@([pscustomobject]@{Name='WinDivert'; State='Running'; PathName=(Join-Path $script:layout.Install 'WinDivert64.sys')})
    $script:clients=@()
    Remove-OwnedDriver $script:layout.Install
    Assert (($script:scCalls -join ',') -eq 'stop WinDivert,delete WinDivert') 'Unused driver owned by this installation is stopped before deletion'

    Reset-Fixture; Install
    Write-File (Join-Path $script:layout.Data 'rules.json') 'delete on uninstall'
    Write-File (Join-Path $script:layout.Data 'setup-results\result.json')
    Write-File (Join-Path $script:layout.Install '.setup-backup\old.exe')
    $taskOutside = Join-Path $taskTestRoot 'unrelated'
    Write-File (Join-Path $taskOutside 'keep.txt')
    foreach ($taskProfile in $script:layout.Profiles) {
        $taskFolders=Get-ProfileFolders $taskProfile
        Write-File (Join-Path $taskFolders.Local 'SpeedLimitFree\WebView2\cache')
        Write-File (Join-Path $taskFolders.Roaming 'SpeedLimitFree.exe\old-cache')
        Write-File (Join-Path $taskFolders.Roaming 'SpeedLimitFree-0.1.2.exe\old-cache')
        Write-File (Join-Path $taskFolders.Local 'OtherApp\keep.txt')
    }
    $taskCache=Join-Path (Get-ProfileFolders $script:layout.Profiles[0]).Local 'SpeedLimitFree\WebView2'
    New-Item -ItemType Junction -Path (Join-Path $taskCache 'external-link') -Target $taskOutside | Out-Null
    Remove-DesktopPackage $script:layout
    Assert (-not $script:registered) 'Uninstall removes service registration'
    Assert (-not (Test-Path -LiteralPath $script:layout.Install) -and -not (Test-Path -LiteralPath $script:layout.Data)) 'Uninstall removes app, backups, rules, and logs'
    Assert (-not (Test-Path -LiteralPath $script:UninstallKey)) 'Uninstall removes Installed Apps registration'
    foreach ($taskProfile in $script:layout.Profiles) {
        $taskFolders=Get-ProfileFolders $taskProfile
        Assert (-not (Test-Path -LiteralPath (Join-Path $taskFolders.Local 'SpeedLimitFree'))) 'Every profile cache is removed'
        Assert (-not (Test-Path -LiteralPath (Join-Path $taskFolders.Roaming 'SpeedLimitFree.exe'))) 'Legacy Wails cache is removed'
        Assert (Test-Path -LiteralPath (Join-Path $taskFolders.Local 'OtherApp\keep.txt')) 'Other app data survives'
    }
    Assert (Test-Path -LiteralPath (Join-Path $taskOutside 'keep.txt')) 'Cleanup removes junction without following its target'
    Fail { Remove-OwnedTree $taskOutside (Join-Path $taskTestRoot 'other-root') } 'Refusing cleanup outside'

    Reset-Fixture; Install
    $taskCache=Join-Path (Get-ProfileFolders $script:layout.Profiles[0]).Local 'SpeedLimitFree\WebView2\locked'
    Write-File $taskCache
    $taskLock=[IO.File]::Open($taskCache,'Open','Read','Read')
    try {
        Fail { Remove-DesktopPackage $script:layout } 'used by another process'
        Assert (Test-Path -LiteralPath $script:UninstallKey) 'Failed cleanup keeps Installed Apps entry for retry'
        Assert (Test-Path -LiteralPath (Join-Path $script:layout.Install 'Uninstall.exe')) 'Failed cache cleanup keeps working uninstaller'
    } finally { $taskLock.Dispose() }
    Remove-DesktopPackage $script:layout
    Assert (-not (Test-Path -LiteralPath $script:UninstallKey)) 'Retry completes cleanup'
    Write-Output "Installer checks passed: $script:checks assertions across $script:cases isolated fixtures."
} finally {
    if (Test-Path -LiteralPath $script:UninstallKey) { Remove-Item -LiteralPath $script:UninstallKey -Recurse -Force }
    Remove-OwnedTree $taskTestRoot $taskTools
}
