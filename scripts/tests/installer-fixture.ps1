# Embedded only in the separate test installer; never included in a release.
param([string]$Action,[string]$Source=$PSScriptRoot,[string]$Version,[int]$Startup)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'installer-common.ps1')
$taskRoot=$env:SPEEDLIMITFREE_INSTALLER_FIXTURE
if (-not $taskRoot -or [IO.Path]::GetFileName($taskRoot) -notlike 'nsis-tests-*') { throw 'Missing isolated installer fixture.' }
$script:UninstallKey='HKCU:\' + $env:SPEEDLIMITFREE_INSTALLER_FIXTURE_KEY
$taskState=Join-Path $taskRoot 'service-fixture.json'
$script:state=if(Test-Path -LiteralPath $taskState){Get-Content -LiteralPath $taskState -Raw | ConvertFrom-Json}else{[pscustomobject]@{registered=$false;status='Stopped';binary=''}}
$taskOwner=New-Object Security.Principal.SecurityIdentifier('S-1-5-21-1234-1234-1234-1001')
$taskLayout=[pscustomobject]@{
    Install=Join-Path $taskRoot 'installed\SpeedLimitFree'; Data=Join-Path $taskRoot 'data\SpeedLimitFree'
    Programs=Join-Path $taskRoot 'programs'; Desktop=Join-Path $taskRoot 'desktop'
    Profiles=@([pscustomobject]@{SID=$taskOwner.Value;Path=Join-Path $taskRoot "User ' Example"})
}
function Set-ProtectedDirectory([string]$Path,$Owner){New-Item -ItemType Directory -Path $Path -Force | Out-Null}
function Get-InstallOwner($Layout){$taskOwner}
function Close-InstalledDesktop([string]$Install){}
function Get-ManagedService{if($script:state.registered){[pscustomobject]@{Status=$script:state.status;StartType='Automatic'}}}
function Get-ServiceRegistration{if($script:state.registered){[pscustomobject]@{State=$script:state.status;StartMode='Auto';PathName=$script:state.binary;ProcessId=0}}}
function Set-ServiceRegistration([string]$Binary,[string]$StartMode){$script:state.registered=$true;$script:state.binary=$Binary}
function Start-Service{$script:state.status='Running'}
function Stop-Service{$script:state.status='Stopped'}
function Set-ServiceExtras([string]$Install){[IO.File]::WriteAllText((Join-Path $taskLayout.Programs 'SpeedLimitFree.lnk'),'fixture shortcut')}
function Get-CimInstance{param($ClassName,$Filter);if($ClassName -eq 'Win32_SystemDriver'){return};throw 'Unexpected non-isolated CIM query'}
function sc.exe{param($Action,$Name);$global:LASTEXITCODE=0;if($Action -eq 'delete' -and $Name -eq 'SpeedLimitFree'){$script:state.registered=$false}}
try {
    if($Action -eq 'install'){Install-DesktopPackage $taskLayout $Source $Version ([bool]$Startup)}
    elseif($Action -eq 'uninstall'){Remove-DesktopPackage $taskLayout}
    else{throw 'Unexpected fixture action'}
    Write-Output "Fixture $Action completed"
} catch { Write-Output $_.Exception.Message;exit 1 }
finally{$script:state | ConvertTo-Json | Set-Content -LiteralPath $taskState -Encoding UTF8}
