param(
    [Parameter(Mandatory=$true)][ValidateSet('install','uninstall','launch')][string]$Action,
    [string]$Source = $PSScriptRoot,
    [ValidatePattern('^\d+\.\d+\.\d+$')][string]$Version = '0.3.0',
    [ValidateSet(0,1)][int]$Startup = 0
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'installer-common.ps1')
$taskMutex = $null; $taskLocked = $false
try {
    $taskLayout = Get-InstallerLayout
    if ($Action -eq 'launch') {
        # The desktop Explorer launches as its normal user, not the setup administrator.
        $taskShell = New-Object -ComObject Shell.Application
        $taskWindows = $taskShell.Windows()
        $taskWindowHandle = 0
        $taskDesktop = $taskWindows.FindWindowSW(0, 0, 8, [ref]$taskWindowHandle, 1)
        if (-not $taskDesktop) { throw 'Open SpeedLimitFree from the Start menu.' }
        $taskDesktop.Document.Application.ShellExecute((Join-Path $taskLayout.Install 'SpeedLimitFree.exe'), '', $taskLayout.Install, 'open', 1)
        exit 0
    }
    if (-not ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Administrator access is required for installation or removal.' }
    $taskMutex = New-Object Threading.Mutex($false, 'Global\SpeedLimitFree.Setup')
    try { $taskLocked = $taskMutex.WaitOne(0) } catch [Threading.AbandonedMutexException] { $taskLocked = $true }
    if (-not $taskLocked) { throw 'Another setup or service operation is running. Wait for it to finish and retry.' }
    if ($Action -eq 'install') {
        $taskOwner = Get-InstallOwner $taskLayout
        $taskRuntimeKey = 'Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}'
        $taskRuntime = @(
            (Get-ItemProperty -LiteralPath ('HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}') -ErrorAction SilentlyContinue).pv
            (Get-ItemProperty -LiteralPath ('Registry::HKEY_USERS\' + $taskOwner.Value + '\' + $taskRuntimeKey) -ErrorAction SilentlyContinue).pv
        ) | Where-Object { $_ -and $_ -ne '0.0.0.0' }
        if (-not $taskRuntime) {
            $taskBootstrap = Join-Path $Source 'MicrosoftEdgeWebview2Setup.exe'
            $taskSignature = Get-AuthenticodeSignature -LiteralPath $taskBootstrap
            if ($taskSignature.Status -ne 'Valid' -or $taskSignature.SignerCertificate.Subject -notlike '*O=Microsoft Corporation*') { throw 'The Microsoft WebView2 setup signature could not be verified.' }
            Write-Output 'Installing Microsoft WebView2. An internet connection is required for this step.'
            $taskChild = Start-Process -FilePath $taskBootstrap -ArgumentList @('/silent','/install') -PassThru -Wait -WindowStyle Hidden
            if ($taskChild.ExitCode -ne 0) { throw "WebView2 installation failed (code $($taskChild.ExitCode)). The application has not been installed." }
        }
        Install-DesktopPackage $taskLayout $Source $Version ([bool]$Startup)
        Write-Output 'SpeedLimitFree is installed and the service is running. Existing rules were kept.'
    } else {
        Remove-DesktopPackage $taskLayout
        Write-Output 'SpeedLimitFree, its service, saved rules, settings, logs, caches, and shortcuts were removed.'
    }
} catch { Write-Output $_.Exception.Message; exit 1 }
finally { if ($taskLocked) { $taskMutex.ReleaseMutex() }; if ($taskMutex) { $taskMutex.Dispose() } }
