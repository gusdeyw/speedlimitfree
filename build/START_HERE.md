# SpeedLimitFree 0.3.0

Created by [gusdeyw](https://github.com/gusdeyw).

For a standard Windows installation, run **SpeedLimitFree-0.3.0-Setup.exe** and approve the administrator prompt. The installer adds the desktop, automatic traffic service, Start menu shortcut, and Windows Installed Apps entry. Startup in the tray is optional. Missing WebView2 is installed using Microsoft's bootstrapper (internet required only for that step).

To update or repair, run the same or a newer installer. Existing limits are preserved. Setup closes running SpeedLimitFree tray windows and restores previous files/service configuration if the update fails.

For the ZIP distribution:

1. Keep all files in this folder together.
2. Open **SpeedLimitFree.exe**.
3. Open **Settings → Install / repair** and complete the Windows administrator prompt.
4. Return to **Applications**, find an app, and click its download or upload limit to set a speed. Expand a row for individual processes. Double-click a row for rule details, or open **Saved limits** to manage rules for closed applications.

The interface follows the Windows app theme, including live light/dark changes. Application icons load from installed executables and stay in memory; no process icon images are saved. Unavailable icons use a generic fallback.

An existing service can be used immediately. Quit the old desktop from its tray before opening this build. Use Install / repair if you also want to update the installed Start menu copy.

Closing X hides the desktop to the system tray beside the Windows clock. Click the speedometer icon to reopen. Right-click for Open, Pause/Resume limits, and Quit SpeedLimitFree. The icon may be inside the ^ overflow.

Settings now has Start, Stop, Restart, and Install / repair, plus separate Windows service and traffic engine status and recent logs. Stop keeps saved rules. Wait for the operation result after accepting the administrator prompt. Repeated setup skips unchanged service files; failed updates attempt rollback. Quit an older desktop from its tray before opening this build.

The desktop runs without administrator privileges. Quit exits the UI and tray; the background service keeps limits active. Launching the executable again restores the existing window.

This build has passed Go, browser, native process-only, and setup regression checks. Repair and traffic-engine startup have been verified on the development machine. Rate accuracy and clean-machine installation still need validation. The app displays an explicit error if the traffic engine cannot start.

To uninstall, use **Windows Settings → Apps → Installed apps → SpeedLimitFree → Uninstall** or the installed **Uninstall.exe**. This permanently deletes all saved limits, settings, logs, backups, app caches, shortcuts, and application files, and removes the service and installer registrations. There is no keep-settings option. Shared Windows components remain. A driver currently used by another application must be released before full removal can finish; errors stay visible and removal can be retried.

For manual setup, run `install-service.ps1` in an administrator PowerShell terminal. `uninstall-service.ps1` performs the same complete removal, including saved rules. It does not delete your downloaded ZIP, source checkout, or Windows installation history.

WinDivert is a separate upstream component. Keep its LICENSE and THIRD_PARTY_NOTICES.md with this distribution.
