# Windows releases

Every successful push to `main` builds and publishes a GitHub release. You can also run **Actions > Windows build and release > Run workflow** on `main`. Pull requests build and test without publishing.

The release includes three downloads:

- `SpeedLimitFree-Setup.exe`: recommended Windows 11 x64 installer.
- `SpeedLimitFree-windows-x64.zip`: complete application and service files for manual setup.
- `SHA256SUMS.txt`: checksums for both downloads.

Release notes link directly to these assets. The README uses `/releases/latest/download/` URLs, which follow the latest release without changing the README for every version. GitHub also creates source archives; these are not the application downloads.

## Versioning

`wails.json` contains the base version. CI adds its workflow run number minus one to the base patch number: base `0.3.0` gives `0.3.0`, `0.3.1`, and so on. Failed builds and pull requests can leave gaps; version numbers are never reused for different workflow runs. The same version is stamped into the desktop, installer, frontend package metadata, and packaged startup guide. These generated edits stay in CI and do not create commits or trigger another build.

Rerunning a failed job reuses that run's version. Uploads happen in a draft; the release is marked Latest only after all files upload. A rerun leaves already published assets intact. To ship a correction, push a new commit or dispatch a new workflow run. Do not reset or decrease the base version; when advancing a minor version, update the base in `wails.json` and the checked-in frontend/package documentation together.

## Checks and permissions

The Windows build runs Svelte/TypeScript checks, Go tests and vet, service setup regression checks, browser tests, and the compiled NSIS lifecycle fixtures. Tests simulate SCM and driver changes in isolated fixtures; they do not qualify live bandwidth enforcement or elevated clean-machine installation. Release notes disclose that the app is unsigned and these validations remain outstanding.

Only the publish job has `contents: write`, using GitHub's built-in token. No personal token or extra repository secret is required. Actions are pinned to commit hashes. Concurrent main builds publish sequentially. GitHub may replace a pending run with a newer push; only builds that actually finish all checks publish.

For local packaging after `scripts/build.ps1`, run `scripts/package-release.ps1`. Outputs are in ignored `build/release/`. The script checks executable version/publisher metadata, uses an explicit payload allowlist, and writes checksums and release notes.
