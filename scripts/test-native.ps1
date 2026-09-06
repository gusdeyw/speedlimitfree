$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
Set-Location -LiteralPath $taskRoot
if (-not (Get-Command go -ErrorAction SilentlyContinue)) { $env:PATH = 'D:\Go\bin;' + $env:PATH }
$taskTools = Join-Path $taskRoot '.tools'
New-Item -ItemType Directory -Path $taskTools -Force | Out-Null
# Wails deliberately removes external WebView2 debug arguments. A local copy
# enables CDP only in this test binary; the module cache and release are untouched.
$taskModule = (go list -m -f '{{.Dir}}' github.com/wailsapp/wails/v2)
$taskOriginal = Join-Path $taskModule 'internal\frontend\desktop\windows\frontend.go'
$taskModuleCopy = Join-Path $taskTools 'wails-smoke'
if (-not (Test-Path -LiteralPath $taskModuleCopy)) { Copy-Item -LiteralPath $taskModule -Destination $taskModuleCopy -Recurse }
$taskOverlaySource = Join-Path $taskModuleCopy 'internal\frontend\desktop\windows\frontend.go'
$taskContent = [IO.File]::ReadAllText($taskOriginal)
$taskNeedle = 'chromium.DataPath = opts.WebviewUserDataPath'
if (-not $taskContent.Contains($taskNeedle)) { throw 'Wails test overlay requires review for this version.' }
$taskContent = $taskContent.Replace($taskNeedle, $taskNeedle + "`n" + '        chromium.AdditionalBrowserArgs = append(chromium.AdditionalBrowserArgs, "--remote-debugging-port=9227")')
(Get-Item -LiteralPath $taskOverlaySource).IsReadOnly = $false
[IO.File]::WriteAllText($taskOverlaySource, $taskContent)
Copy-Item -LiteralPath go.mod -Destination .tools\smoke.mod -Force
Copy-Item -LiteralPath go.sum -Destination .tools\smoke.sum -Force
go mod edit -modfile .tools/smoke.mod "-replace=github.com/wailsapp/wails/v2=$taskModuleCopy"
$taskPipe = '\\.\pipe\SpeedLimitFree.native-test.' + $PID
$taskLinkFlags = '-X speedlimitfree/internal/contracts.PipeName=' + $taskPipe + ' -X speedlimitfree/internal/desktop.DataDirectory=SpeedLimitFree-native-test'
go build -modfile .tools/smoke.mod -tags desktop,production -ldflags ("-H=windowsgui " + $taskLinkFlags) -o .tools/SpeedLimitFree-smoke.exe .
if ($LASTEXITCODE) { throw 'Native test build failed.' }
go build -ldflags $taskLinkFlags -o .tools/SpeedLimitFree-service-smoke.exe ./cmd/service
if ($LASTEXITCODE) { throw 'Test service build failed.' }
go build -ldflags $taskLinkFlags -o .tools/limiter-debug-smoke.exe ./cmd/limiter-debug
if ($LASTEXITCODE) { throw 'Test diagnostics build failed.' }
$taskService = $null; $taskDesktop = $null
$taskPreviousPID = $env:SPEEDLIMITFREE_NATIVE_PID
$taskPreviousDebug = $env:SPEEDLIMITFREE_NATIVE_DEBUG
try {
    $taskService = Start-Process -FilePath (Join-Path $taskTools 'SpeedLimitFree-service-smoke.exe') -ArgumentList @('--monitor-only','--data',('"' + (Join-Path $taskTools 'native-test-data') + '"')) -PassThru -WindowStyle Hidden -RedirectStandardOutput (Join-Path $taskTools 'native-service.out.log') -RedirectStandardError (Join-Path $taskTools 'native-service.err.log')
    $taskDesktop = Start-Process -FilePath (Join-Path $taskTools 'SpeedLimitFree-smoke.exe') -ArgumentList '--tray' -PassThru -WindowStyle Hidden
    $env:SPEEDLIMITFREE_NATIVE_PID = [string]$taskDesktop.Id
    $env:SPEEDLIMITFREE_NATIVE_DEBUG = Join-Path $taskTools 'limiter-debug-smoke.exe'
    $taskReady = $false
    for ($taskAttempt = 0; $taskAttempt -lt 30; $taskAttempt++) {
        try { Invoke-RestMethod 'http://127.0.0.1:9227/json/version' -TimeoutSec 1 | Out-Null; $taskReady = $true; break } catch { Start-Sleep -Milliseconds 300 }
    }
    if (-not $taskReady) { throw 'WebView2 debug target did not start.' }
    Push-Location frontend
    try { node tests/native-smoke.mjs; if ($LASTEXITCODE) { throw 'Native smoke checks failed.' } } finally { Pop-Location }
} finally {
    $env:SPEEDLIMITFREE_NATIVE_PID = $taskPreviousPID
    $env:SPEEDLIMITFREE_NATIVE_DEBUG = $taskPreviousDebug
    foreach ($taskProcess in @($taskDesktop, $taskService)) {
        if ($taskProcess -and -not $taskProcess.HasExited) { Stop-Process -Id $taskProcess.Id -ErrorAction SilentlyContinue }
    }
    . (Join-Path $PSScriptRoot 'installer-common.ps1')
    foreach ($taskCache in @(
        [pscustomobject]@{ Path=(Join-Path $env:LOCALAPPDATA 'SpeedLimitFree-native-test'); Parent=$env:LOCALAPPDATA }
        [pscustomobject]@{ Path=(Join-Path $env:APPDATA 'SpeedLimitFree-smoke.exe'); Parent=$env:APPDATA }
    )) {
        $taskRetry = 0
        while ($true) {
            try { Remove-OwnedTree $taskCache.Path $taskCache.Parent; break }
            catch { if (++$taskRetry -ge 8) { throw }; Start-Sleep -Milliseconds 500 }
        }
    }
}
