import { chromium } from '@playwright/test';
import { mkdir, writeFile } from 'node:fs/promises';
import { execFileSync } from 'node:child_process';

// Run against a locally launched Wails app with WebView2 remote debugging enabled.
const browser = await chromium.connectOverCDP('http://127.0.0.1:9227');
function trayAction(action) {
  const output = execFileSync(
    'powershell.exe',
    [
      '-NoProfile',
      '-File',
      '../scripts/tray-probe.ps1',
      '-DesktopPID',
      process.env.SPEEDLIMITFREE_NATIVE_PID,
      '-Action',
      action,
    ],
    { encoding: 'utf8', windowsHide: true },
  );
  return action === 'status' ? JSON.parse(output) : undefined;
}
async function waitForTray(check) {
  for (let i = 0; i < 10; i++) {
    const state = trayAction('status');
    if (check(state)) return state;
    await new Promise((r) => setTimeout(r, 200));
  }
  throw new Error('Native tray state did not reach the expected value');
}
try {
  const context = browser.contexts()[0];
  const page = context.pages()[0] ?? (await context.waitForEvent('page'));
  await waitForTray((s) => s.trayIcon && s.windowFound && !s.windowVisible);
  trayAction('open');
  await waitForTray((s) => s.trayIcon && s.windowVisible);
  await page.waitForFunction(() => !!window.go?.desktop?.App, { timeout: 20000 });
  await page.waitForFunction(async () => (await window.go.desktop.App.Snapshot()).engine === 'monitor', {
    timeout: 20000,
  });
  const result = await page.evaluate(async () => {
    const api = window.go.desktop.App;
    const before = await api.Snapshot();
    const serviceStatus = await api.ServiceStatus();
    if (!serviceStatus.state || !serviceStatus.logPath)
      throw new Error('Native Windows service diagnostics failed');
    if (!before.connected || before.processes.length < 1) throw new Error('Real process discovery failed');
    const target = before.processes.find((p) => p.name === 'SpeedLimitFree-smoke.exe');
    if (!target) throw new Error('Desktop process not found');
    const data = await api.AppIcon(target.path);
    if (!data.startsWith('data:image/png;base64,')) throw new Error('Native executable icon failed');
    const icon = new Image();
    icon.src = data;
    await icon.decode();
    if (icon.naturalWidth !== 32 || icon.naturalHeight !== 32) throw new Error('Unexpected icon dimensions');
    if ((await api.AppIcon('C:\\missing-speedlimitfree-icon.exe')) !== '')
      throw new Error('Missing icon did not fall back');
    const saved = await api.SaveRule({
      id: '',
      name: 'Native smoke test',
      scope: 'application',
      path: target.path,
      pid: 0,
      started: '',
      download: 125000,
      upload: null,
      enabled: true,
    });
    const rule = saved.rules.find((r) => r.name === 'Native smoke test');
    if (!rule || rule.download !== 125000) throw new Error('Native save failed');
    const paused = await api.SetPaused(true);
    if (!paused.paused) throw new Error('Native pause failed');
    await api.SetPaused(false);
    await api.DeleteRule(rule.id);
    await api.SetVisible(false);
    await api.SetVisible(true);
    return {
      connected: before.connected,
      engine: before.engine,
      processCount: before.processes.length,
      save: 'passed',
      pause: 'passed',
      delete: 'passed',
      visibility: 'passed',
      windowsServiceStatus: 'passed',
      startupInTray: 'passed',
      executableIcon: 'passed',
      missingIconFallback: 'passed',
    };
  });
  await mkdir('../docs/screenshots', { recursive: true });
  await page.getByRole('grid').waitFor();
  await page.waitForFunction(() =>
    [...document.querySelectorAll('.process-icon img')].some(
      (img) => img.complete && img.naturalWidth === 32,
    ),
  );
  const originalTheme = await page.evaluate(() =>
    matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light',
  );
  for (const colorScheme of ['light', 'dark']) {
    await page.emulateMedia({ colorScheme });
    const expected = colorScheme === 'dark' ? 'rgb(32, 35, 39)' : 'rgb(255, 255, 255)';
    await page.waitForFunction(
      (color) => getComputedStyle(document.documentElement).backgroundColor === color,
      expected,
    );
    await page.screenshot({ path: `../docs/screenshots/native-${colorScheme}.png` });
  }
  await page.emulateMedia({ colorScheme: null });
  result.initialSystemTheme = originalTheme;
  result.liveThemeChange = 'passed (WebView2 media emulation)';
  await page.screenshot({ path: '../docs/screenshots/native-monitor.png' });
  await waitForTray((s) => s.trayWindow && s.trayIcon && s.windowFound);
  trayAction('close');
  await waitForTray((s) => s.trayWindow && s.trayIcon && s.windowFound && !s.windowVisible);
  // Page execution still works while hidden, but periodic statistics must stop.
  const hiddenEvents = await page.evaluate(async () => {
    let count = 0;
    const off = window.runtime.EventsOn('snapshot', () => count++);
    await new Promise((r) => setTimeout(r, 1200));
    off();
    return count;
  });
  if (hiddenEvents !== 0) throw new Error('Hidden-to-tray UI continued receiving statistics');
  execFileSync('../.tools/SpeedLimitFree-smoke.exe', [], { windowsHide: true, timeout: 10000 });
  await waitForTray((s) => s.trayIcon && s.windowVisible);
  result.secondInstanceRestore = 'passed';
  trayAction('close');
  await waitForTray((s) => s.trayIcon && !s.windowVisible);
  trayAction('open');
  await waitForTray((s) => s.trayIcon && s.windowVisible);
  trayAction('pause');
  await page.waitForFunction(async () => (await window.go.desktop.App.Snapshot()).paused);
  trayAction('resume');
  await page.waitForFunction(async () => !(await window.go.desktop.App.Snapshot()).paused);
  result.trayIcon = 'passed';
  result.closeToTray = 'passed';
  result.restoreFromTray = 'passed';
  result.pauseFromTray = 'passed';
  result.hiddenStatistics = 'passed';
  trayAction('quit');
  await waitForTray((s) => !s.trayWindow && !s.windowFound);
  const afterQuit = JSON.parse(
    execFileSync(process.env.SPEEDLIMITFREE_NATIVE_DEBUG, ['--status'], {
      encoding: 'utf8',
      windowsHide: true,
    }),
  );
  if (!afterQuit.connected) throw new Error('Quitting the desktop stopped the service');
  result.quitFromTray = 'passed';
  result.serviceSurvivesQuit = 'passed';
  await writeFile('../docs/native-smoke.json', JSON.stringify(result, null, 2) + '\n');
  console.log(JSON.stringify(result));
} finally {
  await browser.close();
}
