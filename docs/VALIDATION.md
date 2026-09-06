# Validation record

## Environment

- Date: 2026-09-06.
- Windows x64 development machine; session is not elevated.
- Node.js 24.15.0; npm 11.12.1.
- Go module target 1.25.0, Wails 2.15.0, Svelte 5.57.0, Vite 8.2.2, TypeScript 6.0.3.
- WinDivert 2.2.2-A x64. Windows Authenticode verification returned `Valid` for the bundled driver.
- Official archive SHA-256: `63cb41763bb4b20f600b6de04e991a9c2be73279e317d4d82f237b150c5f3f15`.

## Completed checks

- Branding update: the shared green speedometer is visible in the app header and Settings. Icons extracted from the compiled desktop and installer match the tray artwork. Windows file details report `gusdeyw`, and Settings links to `https://github.com/gusdeyw`. The frontend checks, 16 browser checks, Go tests/vet, 16 service setup assertions, and 40 installer assertions pass after this update. Installer/ZIP hashes in `installer-validation.json` refer to the rebuilt branded artifacts.
- Go packages compile, including the Windows service, packet DLL wrapper, process tables, and Wails backend.
- All 25 top-level Go tests pass. They cover aggregate bandwidth across four flows, directional independence, per-flow ordering, overload expiration, queue bounds, idle budget retention, and rule edits without unrelated queue release, plus native tray ABI layout, actions, and cleanup.
- Native icon tests extract and decode a real Windows executable's 32 px PNG, check transparency and visible pixels, repeat extraction without accumulating GDI/USER handles, verify path validation and case-insensitive caching, and exercise the 256-entry bound. The implementation does not write icon files.
- Go tests cover rule precedence, PID reuse, validation, duplicate target rejection, atomic file replacement, temporary rule exclusion, and failed-write rollback.
- Windows integration tests resolve a real pre-existing UDP endpoint to the current process and exercise named-pipe save/pause/delete commands and shutdown.
- Packet tests cover IPv4 direction, IPv6 extension parsing, fragments/truncation, and WinDivert host-order address decoding.
- `go vet ./...` passes.
- Svelte/TypeScript checks report zero errors and zero warnings.
- Sixteen Playwright checks pass: process/rule/visibility/layout and service-control regression checks, plus inline directional edits during live snapshots, grouped process overrides, closed-application saved limits, live system theme changes and icon request reuse, stale rule deletion, and disabled process override inheritance. The thousand-process fixture renders fewer than 40 DOM rows. At 920 × 640, the table columns align and the page does not overflow.
- Native Wails/WebView2 smoke testing connects to an isolated process-only Go service. Save, pause/resume, delete, and visibility commands cross the real Wails and named-pipe boundaries. The extended test verifies tray icon registration, native window close-to-tray, restoration from the tray and a second launch, zero hidden statistics events, pause/resume from tray commands, explicit desktop quit, and continued service availability after quit. It also loads and decodes a real executable icon through the bridge, checks missing-icon fallback, and switches the WebView2 color-scheme media preference live. Windows theme settings themselves are left unchanged; native frame preference handling uses Wails' built-in SystemDefault. See `native-smoke.json` for the latest results and observed process count (307 in the 0.2.0 run).
- PowerShell scripts parse successfully. Setup regression tests pass 16 assertions across five isolated fixtures with real staging, hashes, file locks, and rollback, and fake SCM/ACL operations. Checks include unchanged running setup, idempotent start/stop, incomplete packages, locked desktop/core files, failed startup restoration, and saved-rule retention.
- Installer 0.3.0 transaction/cleanup checks pass 40 assertions across seven isolated fixtures: successful install/update, unchanged repair, downgrade rejection, failed fresh install, complete file and metadata rollback, optional startup shortcuts, shared-driver protection, multi-profile/legacy cache removal, junction safety, and retry after locked files.
- Separately compiled NSIS fixture executables pass 18 assertions for native install, upgrade, rejected downgrade, failed uninstall, and successful full-cleanup retry, including paths with spaces. The test uses real NSIS extraction/uninstaller execution, filesystem operations, and isolated HKCU metadata; SCM and driver calls are fixtures. Release installers contain the production helper and request administrator access.
- Native Wails 0.3.0 testing passes startup hidden in the tray using `--tray`, restoration, all existing service-boundary operations, icons/themes, and explicit quit (331 observed processes). Its WebView data is isolated from the application's normal `%LOCALAPPDATA%\SpeedLimitFree\WebView2` directory.
- The NSIS 3.12 compiler builds the release installer successfully. The bundled Microsoft WebView2 bootstrapper has a valid Microsoft Authenticode signature. Explorer's desktop automation object is accessible for normal-user launch. The final installer itself is an unsigned development build; elevated clean-machine install/uninstall, different-account UAC, and actual WinDivert unload remain unqualified.
- Go tests verify matching setup operation IDs, process exit codes, failed results, bounded diagnostic logs, native ShellExecuteEx ABI layout, reading SCM without elevation, and refusing to report Running when saved rules cannot load. Native Wails testing also reads Windows service status through the actual bridge.
- The elevated 0.1.2 repair helper was executed successfully on the existing installation on 2026-09-06. It returned a verified successful result; subsequent SCM and IPC checks reported service Running and traffic engine running. The original saved rules remained unchanged (zero rules, limits not paused). This verifies local repair/startup, not throughput accuracy or clean-machine installation. Stop/restart recovery under live traffic and native UAC cancellation remain unqualified.
- The production Windows desktop executable builds successfully. The 0.2.0 UI JavaScript is approximately 133 kB uncompressed / 40 kB gzip, plus approximately 15 kB CSS. These asset sizes are not RAM measurements. No frontend dependencies were added for the redesign.

The browser screenshots `screenshots/activity.png`, `screenshots/activity-dark.png`, `screenshots/activity-small.png`, and `screenshots/rule-editor.png` use synthetic process/icon fixtures. `screenshots/native-light.png`, `screenshots/native-dark.png`, and `screenshots/native-monitor.png` show real processes and Windows executable icons in the process-only native test. Neither test mode demonstrates live bandwidth enforcement.

## Findings addressed during implementation

- Fixed an overload fairness bug where expired packets advanced the scheduling cursor and favored particular flows.
- Prevented incoming packets from continually resetting the scheduling timer and delaying queue service.
- Preserved unrelated queued traffic and token balances when rules are edited.
- Moved persistent file writes out of the packet-processing state lock.
- Indexed rule matching instead of scanning every saved rule for each packet.
- Cached process enumeration when the service is unavailable.
- Constrained the desktop activity layout so the process list scrolls inside the available window space.
- Kept monitor/offline rates explicitly unavailable and saved rules pending.
- Added a Windows API tray icon with its own sleeping message loop, menu-triggered service refresh, Explorer recreation handling, and explicit Quit. The UI closes to the tray only while a working icon can restore it.
- Isolated native and IPC tests from the installed service so testing never pauses or changes the user's active rules.
- Replaced fire-and-forget setup with a serialized elevated helper whose process exit and operation result are both checked. Added Windows service state, separate traffic engine state, Start/Stop/Restart, and recent logs to Settings.
- Staged changed service files and backups before stopping; skipped identical files; added rollback after failed updates and a nonfatal warning for a locked desktop executable.
- Reported SCM Running only after saved rules load and the control pipe is listening, and added pending-state progress and startup/shutdown logging.
- Replaced the dashboard layout with a 32 px application/process table, inline directional limit editor, saved-limit view, and optional details/settings panels.
- Preserved a live edit's input and the other direction's latest limit; prevented stale deletion from silently recreating a rule; kept disabled process overrides disabled while showing their active inherited application limits.
- Added system light/dark CSS and native frame appearance, lazy memory-only executable icons, and matching scrollbar gutters for header/body alignment.

## Required before production qualification

These checks cannot be claimed from an unelevated session:

1. Run live TCP upload/download shaping against a controlled remote endpoint. Compare packet accounting and application payload rates over 30-second intervals after warm-up.
2. Repeat with multiple processes/connections, existing sockets, UDP, QUIC, IPv6, and rapid process/connection churn. Verify uncertain attribution is surfaced.
3. Measure CPU, all associated WebView2 memory, throughput, latency, and drops with no interception, interception without shaping, and active shaping. Repeat with the desktop open, hidden, and exited.
4. Perform a 30-minute stress run and verify stable memory and bounded queues. Include many simultaneous rules and adversarial packet sizes.
5. Verify graceful stop, forced service termination, recovery, and whether active connections recover after driver handles close.
6. Test clean-machine install/uninstall, reboot, sleep/resume, adapter changes, multiple users, unauthorized local/remote pipe clients, and VPN/firewall interaction.
7. Verify driver compatibility on target Secure Boot and memory-integrity configurations and complete release signing/distribution checks.

No performance guarantee or production-readiness claim is made until these measurements are recorded.
