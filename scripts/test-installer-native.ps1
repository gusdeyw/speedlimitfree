$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'installer-common.ps1')
$taskRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$taskTools=Join-Path $taskRoot '.tools'
$taskFixture=Join-Path $taskTools ('nsis-tests-' + [guid]::NewGuid().ToString('N') + ' with spaces')
$taskRegistry='Software\SpeedLimitFree.InstallerFixture.' + [guid]::NewGuid().ToString('N')
$taskOldRoot=$env:SPEEDLIMITFREE_INSTALLER_FIXTURE; $taskOldKey=$env:SPEEDLIMITFREE_INSTALLER_FIXTURE_KEY
$taskPayload=Join-Path $taskFixture 'payload'
$taskCompiler=Join-Path $taskTools 'nsis-3.12\makensis.exe'
$script:checks=0
function Assert([bool]$Condition,[string]$Message){if(-not $Condition){throw "ASSERT: $Message"};$script:checks++}
function Run-Setup([string]$File,[string]$Arguments,[int]$Expected=0){$taskChild=Start-Process -FilePath $File -ArgumentList $Arguments -PassThru -WindowStyle Hidden; if(-not $taskChild.WaitForExit(60000)){throw 'Fixture installer timed out'}; Assert ($taskChild.ExitCode -eq $Expected) "Installer exit code: $($taskChild.ExitCode), expected $Expected"}
function Compile([string]$Version){
    $taskOutput=Join-Path $taskFixture ("setup-$Version.exe")
    & $taskCompiler /V2 "/DVERSION=$Version" "/DPAYLOAD=$taskPayload" "/DBOOTSTRAPPER=$(Join-Path $taskTools 'MicrosoftEdgeWebview2Setup.exe')" "/DOUTPUT=$taskOutput" "/DFIXTURE=$taskFixture" "/DFIXTURE_KEY=$taskRegistry" (Join-Path $taskRoot 'build\windows\installer\project.nsi')
    if($LASTEXITCODE){throw 'Fixture NSIS compilation failed'}
    return $taskOutput
}
try {
    foreach($taskDir in @($taskPayload,(Join-Path $taskFixture 'programs'),(Join-Path $taskFixture 'desktop'))){New-Item -ItemType Directory -Path $taskDir -Force | Out-Null}
    foreach($taskFile in $script:PayloadFiles){
        if($taskFile -eq 'Uninstall.exe'){continue}
        if($taskFile -like '*.ps1' -or $taskFile -like '*.cs'){Copy-Item -LiteralPath (Join-Path $PSScriptRoot $taskFile) -Destination $taskPayload}
        else{[IO.File]::WriteAllText((Join-Path $taskPayload $taskFile),'fixture version one')}
    }
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'tests\installer-fixture.ps1') -Destination (Join-Path $taskPayload 'installer-actions.ps1') -Force
    $env:SPEEDLIMITFREE_INSTALLER_FIXTURE=$taskFixture; $env:SPEEDLIMITFREE_INSTALLER_FIXTURE_KEY=$taskRegistry
    $taskSetup=Compile '0.3.0'
    Run-Setup $taskSetup '/S /TRAY=1'
    $taskInstall=Join-Path $taskFixture 'installed\SpeedLimitFree'
    $taskData=Join-Path $taskFixture 'data\SpeedLimitFree'
    Assert ((Get-ItemProperty -LiteralPath ('HKCU:\'+$taskRegistry)).DisplayVersion -eq '0.3.0') 'Native setup registers Installed Apps entry'
    Assert ((Get-ItemProperty -LiteralPath ('HKCU:\'+$taskRegistry)).TrayStartup -eq 1) 'Native setup forwards optional tray startup'
    [IO.File]::WriteAllText((Join-Path $taskData 'rules.json'),'rules survive native upgrade')
    [IO.File]::WriteAllText((Join-Path $taskPayload 'SpeedLimitFree.exe'),'fixture version two')
    $taskUpdate=Compile '0.3.1'
    Run-Setup $taskUpdate '/S'
    Assert ((Get-ItemProperty -LiteralPath ('HKCU:\'+$taskRegistry)).DisplayVersion -eq '0.3.1') 'Native upgrade updates metadata'
    Assert ((Get-ItemProperty -LiteralPath ('HKCU:\'+$taskRegistry)).TrayStartup -eq 1) 'Silent upgrade preserves startup preference'
    Assert ([IO.File]::ReadAllText((Join-Path $taskData 'rules.json')) -eq 'rules survive native upgrade') 'Native upgrade preserves rules'
    Assert ([IO.File]::ReadAllText((Join-Path $taskInstall 'SpeedLimitFree.exe')) -eq 'fixture version two') 'Native upgrade replaces payload'
    Run-Setup $taskSetup '/S' 1
    Assert ((Get-ItemProperty -LiteralPath ('HKCU:\'+$taskRegistry)).DisplayVersion -eq '0.3.1') 'Rejected downgrade keeps current installation'
    $taskCache=Join-Path $taskFixture "User ' Example\AppData\Local\SpeedLimitFree\WebView2"
    New-Item -ItemType Directory -Path $taskCache -Force | Out-Null
    [IO.File]::WriteAllText((Join-Path $taskCache 'cache'),'remove me')
    # Run a copy outside the install root with _?= so the parent can wait for its actual exit.
    $taskUninstaller=Join-Path $taskFixture 'uninstall-run.exe'
    Copy-Item -LiteralPath (Join-Path $taskInstall 'Uninstall.exe') -Destination $taskUninstaller
    $taskLock=[IO.File]::Open((Join-Path $taskCache 'cache'),'Open','Read','Read')
    try {
        Run-Setup $taskUninstaller ('/S _?=' + $taskInstall) 1
        Assert (Test-Path -LiteralPath (Join-Path $taskInstall 'Uninstall.exe')) 'Native cleanup failure preserves retry uninstaller'
        Assert (Test-Path -LiteralPath ('HKCU:\'+$taskRegistry)) 'Native cleanup failure retains Installed Apps entry'
    } finally { $taskLock.Dispose() }
    Run-Setup $taskUninstaller ('/S _?=' + $taskInstall)
    Assert (-not (Test-Path -LiteralPath $taskInstall)) 'Native uninstaller removes complete application directory'
    Assert (-not (Test-Path -LiteralPath $taskData) -and -not (Test-Path -LiteralPath $taskCache)) 'Native uninstaller removes saved rules and WebView cache'
    Assert (-not (Test-Path -LiteralPath ('HKCU:\'+$taskRegistry))) 'Native uninstaller removes Installed Apps entry'
    Assert (-not (Get-Content -LiteralPath (Join-Path $taskFixture 'service-fixture.json') -Raw | ConvertFrom-Json).registered) 'Native uninstaller unregisters fixture service'
    Write-Output "Native NSIS checks passed: $script:checks assertions; install, upgrade, full uninstall. Windows SCM and driver calls were isolated."
} finally {
    $env:SPEEDLIMITFREE_INSTALLER_FIXTURE=$taskOldRoot; $env:SPEEDLIMITFREE_INSTALLER_FIXTURE_KEY=$taskOldKey
    if(Test-Path -LiteralPath ('HKCU:\'+$taskRegistry)){Remove-Item -LiteralPath ('HKCU:\'+$taskRegistry) -Recurse -Force}
    Remove-OwnedTree $taskFixture $taskTools
}
