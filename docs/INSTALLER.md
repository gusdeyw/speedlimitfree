# Windows installer — 0.3.0

The primary distribution is `SpeedLimitFree-0.3.0-Setup.exe`, built with NSIS 3.12. The app remains Go/Wails/Svelte. There is no resident installer process or new frontend dependency.

Release policy: publish the newest completed version's installer as the main download. The initial GitHub release will use the current branded 0.3.0 build; earlier development versions are not release candidates. The extracted folder and ZIP are local packaging alternatives for that same version. The desktop, service, and diagnostic executables are components bundled by Setup, not separate application versions.

## Installation and updates

- Windows 11 on native AMD64; fixed destination `%ProgramFiles%\SpeedLimitFree` and protected service data `%ProgramData%\SpeedLimitFree`.
- One administrator prompt. A fresh install identifies the interactive Explorer account as the IPC owner, even if a different administrator accepts UAC. Updates retain the previous service owner.
- Stage every installer payload file, including the desktop, helpers, and embedded uninstaller. Stop the service only after staging succeeds. Restore previous files and configuration on failure; preserve rules across updates. Reject version downgrades.
- Register the application in Windows Installed Apps, create the Start menu shortcut, and optionally create the owner's startup shortcut using `--tray`. Closing to the tray, showing an existing instance, and service independence remain supported.
- Launch the desktop through the existing Explorer shell, without inheriting the installer's administrator token.
- Detect machine-wide WebView2 or the actual owner's per-user runtime. Bundle Microsoft's signed bootstrapper and use it only when the runtime is absent. Network access is required for that prerequisite installation.
- The UI's checked Start/Stop/Restart/Install-repair operations share the existing global setup mutex with the installer and uninstaller.

`/S` performs silent installation. `/TRAY=1` enables tray startup; `/TRAY=0` disables it. Upgrades keep the previous choice if no override is given. The installed `Uninstall.exe /S` performs complete removal without a confirmation page.

## Full removal

The user explicitly requested removal of all app-owned state. There is no keep-settings switch.

Uninstall closes SpeedLimitFree tray desktops, stops the service, checks the installed driver, and unregisters the service. It removes all installed files, saved rules, setup logs/results/backups, Start menu/startup shortcuts, the Installed Apps registry key, and app-owned browser caches in local user profiles. New builds use `%LOCALAPPDATA%\SpeedLimitFree\WebView2`; the older Wails `%APPDATA%\SpeedLimitFree.exe` and versioned executable cache folders are also recognized.

Paths are resolved and checked before removal. Cleanup does not follow directory junctions or symlinks. An unsupported app-data redirect outside a profile is rejected for deliberate handling, rather than authorizing elevated deletion at an arbitrary path. Loaded profile folder values are read without expanding variables under the administrator's identity. Unloaded profiles use their standard AppData paths; nonstandard unloaded-profile redirection needs further qualification.

Only a WinDivert registration pointing to this installation's `WinDivert64.sys` is eligible for removal. Before stopping a running driver, a read-only REFLECT handle with NO_INSTALL enumerates other clients. If another client remains, removal stops with an actionable error. Registrations belonging to other applications and the shared WebView2 runtime remain untouched. Windows can require a restart if it cannot unload a driver.

Locked files and incomplete cleanup return a failure code. The Installed Apps entry is retained until cleanup completes, and the embedded uninstaller is kept or restored for retry. Downloaded archives/installers, source files, and Windows-maintained history are outside the app's installation ownership.

## Build and tests

`scripts/build.ps1` builds the app, service, helpers, and installer. `scripts/build-installer.ps1` packages an existing `build/bin` payload. The compiler archive is pinned to SHA-256 `56581F90DB321581C5381193D796FFFCF2D24B2F8FED2160A6C6A3BAA67F2C4F`; its build-only files stay in `.tools`. The Microsoft bootstrapper's Authenticode signature is checked at build time and again if runtime installation is needed.

- `scripts/test-service-setup.ps1`: existing service staging, idempotence, locks, and rollback.
- `scripts/test-installer.ps1`: real file/shortcut/user-registry operations with isolated service and driver fixtures; upgrades, fresh-install failure, complete rollback, shared-driver protection, cross-profile cache removal, junction safety, and retry after locked files.
- `scripts/test-installer-native.ps1`: compiles separate unelevated fixture installers from the same NSIS script, then executes install, upgrade, downgrade rejection, failed uninstall, and successful retry. Production builds contain no fixture helper or test install path.
- `scripts/test-native.ps1`: real Wails/WebView2 and an isolated monitor service, including `--tray` startup, icon/theme behavior, rule operations, explicit quit, and service independence.

The real installed service and saved rules are not used as destructive test fixtures. Clean-machine elevated installation/uninstallation, different-account UAC, actual driver unload while other clients run, reboot recovery, and distribution signing remain release qualification checks.

References: [Wails NSIS support](https://wails.io/docs/guides/windows-installer/), [NSIS scripting](https://nsis.sourceforge.io/Docs/Chapter4.html), [WebView2 distribution](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/distribution), [WinDivert REFLECT and removal](https://reqrypt.org/windivert-doc.html), [launch through Explorer](https://devblogs.microsoft.com/oldnewthing/20131118-00/?p=2643).
