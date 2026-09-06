# Third-party notices

This application uses Go, Wails, Svelte, Vite, Phosphor icons, Microsoft go-winio, and golang.org/x/sys. Their pinned versions are recorded in go.mod, go.sum, and frontend/package-lock.json. Their upstream licenses apply.

The Windows installer is built with [NSIS 3.12](https://nsis.sourceforge.io/) and its standard Modern UI components. NSIS source and licensing are available from its official distribution. The installer bundles the unmodified, Microsoft-signed [WebView2 Evergreen bootstrapper](https://developer.microsoft.com/en-us/microsoft-edge/webview2/), which installs the shared Microsoft runtime only if missing. Microsoft's runtime terms apply; uninstalling SpeedLimitFree does not remove this shared runtime.

## WinDivert

WinDivert 2.2.2-A is dynamically loaded as a separate DLL. It is a third-party packet interception library and kernel driver, copyright its contributors.

- Distribution: https://github.com/basil00/WinDivert/releases/tag/v2.2.2
- Corresponding source: https://github.com/basil00/WinDivert/tree/v2.2.2
- Documentation and license: https://reqrypt.org/windivert-doc.html#license

WinDivert is available under LGPL v3 (or later), with alternative GPL licensing described upstream. The downloaded distribution's LICENSE, COPYING, and COPYING.LESSER files must accompany its binaries when present. The build script copies them into the application directory. Do not remove these notices when redistributing the package.

The application does not modify WinDivert. Its DLL remains replaceable by a compatible build. Review the upstream license requirements before distributing a release.
