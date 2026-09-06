import { test, expect } from '@playwright/test';

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    const w = window as any;
    const processes = [
      'Steam.exe',
      'Chrome.exe',
      'Discord.exe',
      'OneDrive.exe',
      'Spotify.exe',
      'Code.exe',
      'Firefox.exe',
      'Teams.exe',
    ].map((name, i) => ({
      pid: 100 + i,
      started: '123456789012345678',
      name,
      path: `C:\\Apps\\${name}`,
      download: [4_820_000, 1_320_000, 35000, 160000, 70000, 0, 40000, 12000][i],
      upload: [82000, 24000, 18000, 400000, 2000, 0, 3200, 5000][i],
      downloaded: 0,
      uploaded: 0,
      ruleId: i === 0 ? 'steam' : '',
    }));
    const s = {
      version: 1,
      connected: true,
      engine: 'running',
      message: 'Traffic engine connected',
      paused: false,
      processes,
      rules: [
        {
          id: 'steam',
          name: 'Steam.exe',
          path: 'C:\\Apps\\Steam.exe',
          scope: 'application',
          pid: 0,
          started: '',
          download: 5000000,
          upload: 500000,
          enabled: true,
        },
      ],
      download: 6577000,
      upload: 534200,
      unknownBytes: 0,
      dropped: 0,
      queueBytes: 0,
      uptime: 480,
    };
    let callback: ((s: any) => void) | null = null;
    const response = () => JSON.parse(JSON.stringify(s));
    w.__fixture = s;
    w.__emit = () => callback?.(response());
    w.__visibilityCalls = [];
    w.__iconCalls = [];
    w.__service = {
      installed: true,
      state: 'running',
      startup: 'Automatic',
      binary: 'service.exe',
      pid: 42,
      exitCode: 0,
      serviceExitCode: 0,
      logPath: 'C:\\ProgramData\\SpeedLimitFree',
      serviceLog: '',
      setupLog: '',
    };
    w.__serviceActions = [];
    w.runtime = {
      EventsOn: (_name: string, fn: (s: any) => void) => {
        callback = fn;
        return () => (callback = null);
      },
    };
    w.go = {
      desktop: {
        App: {
          Snapshot: async () => response(),
          AppIcon: async (path: string) => {
            w.__iconCalls.push(path);
            if (path.includes('missing')) return '';
            return 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScLbtAAAAABJRU5ErkJggg==';
          },
          SaveRule: async (r: any) => {
            if (w.__failSave) throw new Error('Settings could not be written');
            r.id ||= 'new-rule';
            const i = s.rules.findIndex((x) => x.id === r.id);
            if (i >= 0) s.rules[i] = r;
            else s.rules.push(r);
            for (const p of processes) if (p.path === r.path) p.ruleId = r.enabled ? r.id : '';
            return response();
          },
          DeleteRule: async (id: string) => {
            s.rules = s.rules.filter((r) => r.id !== id);
            return response();
          },
          SetPaused: async (paused: boolean) => {
            s.paused = paused;
            return response();
          },
          SetVisible: async (v: boolean) => {
            w.__visibilityCalls.push(v);
          },
          ChooseExecutable: async () => 'C:\\Apps\\NewApp.exe',
          ServiceStatus: async () => JSON.parse(JSON.stringify(w.__service)),
          ManageService: async (action: string) => {
            w.__serviceActions.push(action);
            if (w.__holdService)
              await new Promise((resolve) => {
                w.__finishService = resolve;
              });
            if (w.__serviceFailure) throw new Error(w.__serviceFailure);
            w.__service.installed = true;
            w.__service.state = action === 'stop' ? 'stopped' : 'running';
            w.__service.pid = action === 'stop' ? 0 : 42;
            s.connected = action !== 'stop';
            s.engine = action === 'stop' ? 'offline' : 'running';
            return {
              success: true,
              message: action === 'stop' ? 'Service stopped. Saved rules are kept.' : 'Service is running.',
            };
          },
        },
      },
    };
  });
  await page.goto('/');
});

test('service controls use Windows state and wait for setup completion', async ({ page }) => {
  await page.evaluate(() => {
    const w = window as any;
    w.__service.state = 'stopped';
    w.__service.pid = 0;
    w.__fixture.connected = false;
    w.__fixture.engine = 'offline';
    w.__holdService = true;
    w.__emit();
  });
  await page.getByRole('button', { name: 'Settings', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Start', exact: true })).toBeEnabled();
  await expect(page.getByRole('button', { name: 'Stop', exact: true })).toBeDisabled();
  await page.getByRole('button', { name: 'Start', exact: true }).click();
  await expect(page.getByRole('status')).toContainText('Waiting for the result');
  await expect(page.getByRole('button', { name: 'Install / repair', exact: true })).toBeDisabled();
  await expect(page.getByText('Service is running.', { exact: true })).toHaveCount(0);
  await page.evaluate(() => (window as any).__finishService());
  await expect(page.getByRole('status')).toContainText('Service is running.');
  await expect(page.getByRole('button', { name: 'Start', exact: true })).toBeDisabled();
  await expect(page.getByRole('button', { name: 'Stop', exact: true })).toBeEnabled();
  expect(await page.evaluate(() => (window as any).__serviceActions)).toEqual(['start']);
});

test('setup failure is visible with logs and no success notice', async ({ page }) => {
  await page.evaluate(() => {
    const w = window as any;
    w.__serviceFailure = 'Update failed. Previous service files and configuration were restored.';
    w.__service.setupLog = 'Access denied copying the service executable.';
  });
  await page.getByRole('button', { name: 'Settings', exact: true }).click();
  await page.getByRole('button', { name: 'Install / repair', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('Previous service files');
  await expect(page.getByRole('status')).toHaveCount(0);
  await page.getByText('Service and setup logs', { exact: true }).click();
  await expect(page.locator('.service-logs').filter({ hasText: 'Service and setup logs' })).toContainText(
    'Access denied copying',
  );
  await expect(page.getByRole('button', { name: 'Stop', exact: true })).toBeEnabled();
});

test('not installed and cancelled operations keep service controls accurate', async ({ page }) => {
  await page.evaluate(() => {
    const w = window as any;
    w.__service.installed = false;
    w.__service.state = 'not installed';
  });
  await page.getByRole('button', { name: 'Settings', exact: true }).click();
  for (const name of ['Start', 'Stop', 'Restart'])
    await expect(page.getByRole('button', { name, exact: true })).toBeDisabled();
  await expect(page.getByRole('button', { name: 'Install / repair', exact: true })).toBeEnabled();
  await page.evaluate(() => {
    (window as any).__serviceFailure =
      'Windows administrator prompt was cancelled. No service changes were made';
  });
  await page.getByRole('button', { name: 'Install / repair', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('cancelled');
  await expect(page.getByRole('button', { name: 'Start', exact: true })).toBeDisabled();
});

test('stop and restart are distinct from pausing limits', async ({ page }) => {
  await page.getByRole('button', { name: 'Settings', exact: true }).click();
  await page.getByRole('button', { name: 'Restart', exact: true }).click();
  await expect(page.getByRole('status')).toContainText('Service is running.');
  await page.getByRole('button', { name: 'Stop', exact: true }).click();
  await expect(page.getByRole('status')).toContainText('Saved rules are kept');
  await expect(page.getByRole('button', { name: 'Start', exact: true })).toBeEnabled();
  expect(await page.evaluate(() => (window as any).__serviceActions)).toEqual(['restart', 'stop']);
  await page.screenshot({ path: '../docs/screenshots/service-settings.png', fullPage: true });
});

test('shows real contract data, searches, and pauses rules', async ({ page }) => {
  await expect(page.getByRole('grid')).toContainText('Steam');
  await page.getByRole('textbox', { name: 'Search applications' }).fill('steam');
  await expect(page.locator('.table-row')).toHaveCount(1);
  await page.getByRole('button', { name: 'Pause limits', exact: true }).click();
  await expect(page.locator('.paused-status')).toBeVisible();
  await page.getByRole('button', { name: 'Resume limits', exact: true }).click();
  await expect(page.locator('.paused-status')).toHaveCount(0);
});

test('edits independent limits and persists saved rule state', async ({ page }) => {
  await page.locator('.table-row .app-name').filter({ hasText: 'Steam' }).dblclick();
  await page.getByRole('button', { name: 'Edit rule for Steam.exe', exact: true }).click();
  await page.getByLabel('Download limit', { exact: true }).fill('3');
  await page.getByLabel('Upload limit', { exact: true }).fill('250');
  await page.getByRole('button', { name: 'Save rule', exact: true }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(page.locator('.table-row').filter({ hasText: 'Steam' })).toContainText('3.00 MB/s');
  await page.getByRole('button', { name: /Saved limits/ }).click();
  await expect(page.locator('.table-row')).toContainText('250.0 KB/s');
  await page.getByRole('checkbox', { name: 'Enable rule for Steam.exe' }).uncheck();
  await expect(page.getByRole('checkbox', { name: 'Enable rule for Steam.exe' })).not.toBeChecked();
});

test('save failure stays visible and does not claim success', async ({ page }) => {
  await page.evaluate(() => {
    (window as any).__failSave = true;
  });
  await page.getByRole('button', { name: 'Download limit for Steam.exe', exact: true }).click();
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  await expect(page.getByRole('dialog')).toBeVisible();
  await expect(page.getByRole('alert')).toContainText('Settings could not be written');
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog')).toHaveCount(0);
});

test('virtualizes a thousand processes and handles empty search', async ({ page }) => {
  await page.evaluate(() => {
    const w = window as any;
    w.__fixture.processes = Array.from({ length: 1000 }, (_, i) => ({
      pid: i + 1000,
      started: '100',
      name: `Process-${String(i).padStart(4, '0')}.exe`,
      path: `C:\\Apps\\${i}.exe`,
      download: 0,
      upload: 0,
      downloaded: 0,
      uploaded: 0,
      ruleId: '',
    }));
    w.__emit();
  });
  await expect(page.getByRole('grid')).toHaveAttribute('aria-rowcount', '1001');
  expect(await page.locator('.table-row').count()).toBeLessThan(40);
  await page.locator('.table-scroll').evaluate((el) => (el.scrollTop = el.scrollHeight));
  await expect(page.getByRole('grid')).toContainText('Process-0999');
  await page.getByRole('textbox', { name: 'Search applications' }).fill('missing-app');
  await expect(page.getByRole('heading', { name: 'No matching applications' })).toBeVisible();
});

test('offline state never presents fabricated rates and pauses subscriptions when hidden', async ({
  page,
}) => {
  await page.evaluate(() => {
    const w = window as any;
    w.__fixture.connected = false;
    w.__fixture.engine = 'offline';
    w.__emit();
    Object.defineProperty(document, 'hidden', { configurable: true, value: true });
    document.dispatchEvent(new Event('visibilitychange'));
  });
  await expect(page.locator('.service-banner')).toContainText('Service offline');
  await expect(page.getByRole('button', { name: 'Add application', exact: true })).toBeDisabled();
  expect(await page.evaluate(() => (window as any).__visibilityCalls.at(-1))).toBe(false);
  await expect(page.locator('.total strong').first()).toHaveText('—');
});

test('captures desktop views without overflow or runtime errors', async ({ page }) => {
  const errors: string[] = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.emulateMedia({ colorScheme: 'light' });
  await expect(page.getByRole('grid')).toBeVisible();
  await page.screenshot({ path: '../docs/screenshots/activity.png', fullPage: true });
  await page.getByRole('button', { name: 'Download limit for Steam.exe', exact: true }).click();
  await page.screenshot({ path: '../docs/screenshots/rule-editor.png', fullPage: true });
  await page.keyboard.press('Escape');
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.screenshot({ path: '../docs/screenshots/activity-dark.png', fullPage: true });
  await page.setViewportSize({ width: 920, height: 640 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  expect(await page.evaluate(() => document.documentElement.scrollHeight <= window.innerHeight)).toBe(true);
  const columnsAligned = await page.evaluate(() => {
    const header = [...document.querySelectorAll('.table-header > div')].map(
      (el) => el.getBoundingClientRect().right,
    );
    const cells = [...document.querySelector('.table-row')!.children];
    return (
      cells.length === header.length &&
      cells.every((el, i) => Math.abs(el.getBoundingClientRect().right - header[i]) <= 1)
    );
  });
  expect(columnsAligned).toBe(true);
  await page.screenshot({ path: '../docs/screenshots/activity-small.png', fullPage: true });
  expect(errors).toEqual([]);
});

test('inline edits keep the other direction current while live statistics update', async ({ page }) => {
  await page.getByRole('button', { name: 'Download limit for Steam.exe', exact: true }).click();
  await page.getByRole('spinbutton', { name: 'Speed limit' }).fill('2');
  await page.evaluate(() => {
    const w = window as any;
    w.__fixture.rules[0].upload = 750000;
    w.__fixture.processes[0].download = 1234;
    w.__emit();
  });
  await expect(page.getByRole('spinbutton', { name: 'Speed limit' })).toHaveValue('2');
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  expect(await page.evaluate(() => (window as any).__fixture.rules[0])).toMatchObject({
    download: 2000000,
    upload: 750000,
  });
  await expect(page.getByRole('button', { name: 'Download limit for Steam.exe', exact: true })).toBeFocused();
  await page.getByRole('button', { name: 'Upload limit for Steam.exe', exact: true }).click();
  await page.getByRole('button', { name: 'Set unlimited', exact: true }).click();
  expect(await page.evaluate(() => (window as any).__fixture.rules[0])).toMatchObject({
    download: 2000000,
    upload: null,
  });
});

test('grouping aggregates traffic and PID edits create temporary overrides', async ({ page }) => {
  await page.evaluate(() => {
    const w = window as any;
    w.__fixture.processes.push({ ...w.__fixture.processes[0], pid: 2020, download: 180000, upload: 18000 });
    w.__emit();
  });
  const parent = page.locator('.table-row').filter({ hasText: 'Steam' });
  await expect(parent).toHaveCount(1);
  await expect(parent).toContainText('5.00 MB/s');
  await expect(parent.locator('.process-id')).toHaveText('2');
  await page.getByRole('button', { name: 'Expand Steam', exact: true }).click();
  await expect(page.locator('.table-row.child')).toHaveCount(2);
  await page.getByRole('button', { name: 'Download limit for Steam.exe PID 2020', exact: true }).click();
  await expect(page.getByRole('dialog')).toContainText('Temporary override');
  await page.getByRole('spinbutton', { name: 'Speed limit' }).fill('1');
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  const rules = await page.evaluate(() => (window as any).__fixture.rules);
  expect(rules).toHaveLength(2);
  expect(rules[0].download).toBe(5000000);
  expect(rules[1]).toMatchObject({
    scope: 'process',
    pid: 2020,
    started: '123456789012345678',
    download: 1000000,
    upload: 500000,
  });
  await page.getByRole('textbox', { name: 'Search applications' }).fill('2020');
  await expect(page.locator('.table-row:not(.child)')).toHaveCount(1);
  await expect(page.locator('.table-row.child').filter({ hasText: 'PID 2020' })).toBeVisible();
});

test('saved limits remain editable when applications exit', async ({ page }) => {
  await page.evaluate(() => {
    const w = window as any;
    w.__fixture.processes = w.__fixture.processes.filter((p: any) => p.name !== 'Steam.exe');
    w.__emit();
  });
  await expect(page.locator('.table-row').filter({ hasText: 'Steam' })).toHaveCount(0);
  await page.getByRole('button', { name: /Saved limits/ }).click();
  await expect(page.locator('.table-row')).toContainText('Not running');
  await page.locator('.app-name').filter({ hasText: 'Steam' }).dblclick();
  await page.getByRole('button', { name: 'Remove rule', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'No saved limits' })).toBeVisible();
});

test('system appearance changes live and executable icons are reused across snapshots', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'light' });
  await expect(page.locator('html')).toHaveCSS('background-color', 'rgb(255, 255, 255)');
  await expect(page.locator('.process-icon img')).toHaveCount(8);
  await page.getByRole('textbox', { name: 'Search applications' }).fill('steam');
  await page.locator('.app-name').filter({ hasText: 'Steam' }).click();
  await page.emulateMedia({ colorScheme: 'dark' });
  await expect(page.locator('html')).toHaveCSS('background-color', 'rgb(32, 35, 39)');
  await expect(page.locator('.table-row.selected')).toHaveCount(1);
  await expect(page.getByRole('textbox', { name: 'Search applications' })).toHaveValue('steam');
  await page.emulateMedia({ colorScheme: 'light' });
  await expect(page.locator('html')).toHaveCSS('background-color', 'rgb(255, 255, 255)');
  await page.getByRole('textbox', { name: 'Search applications' }).fill('');
  await expect(page.locator('.process-icon img')).toHaveCount(8);
  await page.evaluate(() => {
    for (let i = 0; i < 20; i++) (window as any).__emit();
  });
  expect(await page.evaluate(() => (window as any).__iconCalls)).toHaveLength(8);
  await page.evaluate(() => {
    const w = window as any;
    w.__fixture.processes[0].path = 'C:\\Apps\\missing.exe';
    w.__emit();
  });
  await expect(page.locator('.process-icon img')).toHaveCount(7);
  await expect(page.locator('.process-icon svg')).toHaveCount(1);
});

test('stale rule deletion does not silently recreate a limit', async ({ page }) => {
  await page.getByRole('button', { name: 'Download limit for Steam.exe', exact: true }).click();
  await page.evaluate(() => {
    const w = window as any;
    w.__fixture.rules = [];
    w.__emit();
  });
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('changed or was removed');
  expect(await page.evaluate(() => (window as any).__fixture.rules)).toHaveLength(0);
});

test('disabled process overrides display the inherited limit and stay disabled when edited', async ({
  page,
}) => {
  await page.evaluate(() => {
    const w = window as any;
    const process = w.__fixture.processes[0];
    w.__fixture.rules.push({
      ...w.__fixture.rules[0],
      id: 'disabled-override',
      scope: 'process',
      pid: process.pid,
      started: process.started,
      download: 1000000,
      enabled: false,
    });
    w.__emit();
  });
  await page.getByRole('button', { name: 'Expand Steam', exact: true }).click();
  const child = page.locator('.table-row.child');
  await expect(child).toContainText('Inherited');
  const cell = page.getByRole('button', { name: 'Download limit for Steam.exe PID 100', exact: true });
  await expect(cell).toContainText('5.00 MB/s');
  await cell.click();
  await expect(page.getByRole('spinbutton', { name: 'Speed limit' })).toHaveValue('1');
  await expect(page.getByRole('dialog')).toContainText('Saving keeps it disabled');
  await page.getByRole('spinbutton', { name: 'Speed limit' }).fill('2');
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  expect(await page.evaluate(() => (window as any).__fixture.rules[1])).toMatchObject({
    download: 2000000,
    enabled: false,
  });
  await expect(cell).toContainText('5.00 MB/s');
});
