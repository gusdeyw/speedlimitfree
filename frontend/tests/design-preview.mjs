// Verify the standalone design concept, without connecting to the Windows service.
import { chromium } from '@playwright/test';
import { pathToFileURL } from 'node:url';
import { resolve } from 'node:path';
import { writeFile } from 'node:fs/promises';

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1280, height: 820 }, deviceScaleFactor: 1 });
const errors = [];
page.on('pageerror', e => errors.push(e.message));
const assert = (value, message) => { if (!value) throw new Error(message); };
const url = pathToFileURL(resolve('../docs/design/table-concept.html')).href;
try {
  await page.goto(url);
  await page.getByRole('table').waitFor();
  assert(await page.locator('tbody tr').count() === 24, 'Expected all sample applications');
  await page.screenshot({ path: '../docs/design/table-light.png' });
  const layout = await page.evaluate(() => {
    const table = document.querySelector('.table-scroller').getBoundingClientRect();
    const rows = [...document.querySelectorAll('tbody tr')].filter(row => { const r = row.getBoundingClientRect(); return r.top >= table.top && r.bottom <= table.bottom; });
    return { viewport: '1280 × 820', fullRowsVisible: rows.length, tableHeight: table.height, tableTop: table.top, rowHeight: rows[0].getBoundingClientRect().height, tableAreaPercent: Math.round(table.width*table.height/(innerWidth*innerHeight)*100) };
  });
  await page.getByRole('button', { name: 'Download limit for Steam', exact: true }).click();
  await page.getByRole('dialog', { name: 'Download limit', exact: true }).waitFor();
  await page.screenshot({ path: '../docs/design/table-edit-limit.png' });
  await page.getByRole('spinbutton', { name: 'Speed limit', exact: true }).fill('2.5');
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  assert((await page.getByRole('button', { name: 'Download limit for Steam', exact: true }).innerText()).includes('2.50 MB/s'), 'Inline limit did not change');
  await page.getByRole('searchbox', { name: 'Search applications', exact: true }).fill('chrome');
  assert(await page.locator('tbody tr').count() === 1, 'Search did not isolate Chrome');
  await page.getByRole('button', { name: 'Expand Chrome', exact: true }).click();
  assert(await page.locator('tbody tr').count() === 13, 'Application group did not expand');
  await page.getByRole('button', { name: 'Download limit for Chrome.exe PID 4820', exact: true }).click();
  assert((await page.locator('#scope-note').innerText()).includes('Temporary override'), 'Process scope not clear');
  await page.getByRole('button', { name: '1 MB/s', exact: true }).click();
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  assert((await page.locator('tr[data-id="0-0"]').innerText()).includes('Override'), 'Process override was not marked');
  await page.getByRole('button', { name: 'Saved limits', exact: false }).click();
  assert((await page.locator('#rows').innerText()).includes('Chrome'), 'Saved view lost a process-only rule');
  await page.getByRole('button', { name: 'Pause limits', exact: true }).click();
  assert((await page.locator('tr[data-id="18"]').innerText()).includes('Paused'), 'Global pause not reflected');
  await page.goto(url);
  await page.getByRole('button', { name: 'Switch to dark theme', exact: true }).click();
  await page.screenshot({ path: '../docs/design/table-dark.png' });
  await page.getByRole('button', { name: 'Manage service', exact: true }).click();
  await page.getByRole('button', { name: 'Stop', exact: true }).click();
  assert((await page.locator('#service-state').innerText()) === 'Stopped', 'Service preview control failed');
  await page.getByRole('button', { name: 'Close service panel', exact: true }).click();
  assert((await page.locator('tr[data-id="18"]').innerText()).includes('Offline'), 'Unavailable service did not clear live rates');
  await page.goto(url);
  await page.setViewportSize({ width: 920, height: 640 });
  assert(await page.evaluate(() => document.documentElement.scrollWidth === innerWidth), 'Small desktop has horizontal page overflow');
  assert(await page.evaluate(() => document.documentElement.scrollHeight === innerHeight), 'Small desktop has page overflow');
  await page.screenshot({ path: '../docs/design/table-small.png' });
  assert(errors.length === 0, errors.join('\n'));
  const result = { checks: 'passed', ...layout, verified: ['search', 'application expansion', 'inline directional limit', 'process override', 'saved process rules', 'pause state', 'theme switch', 'service simulation', '920 × 640 layout'], runtimeErrors: errors };
  await writeFile('../docs/design/preview-checks.json', JSON.stringify(result, null, 2) + '\n');
  console.log(JSON.stringify(result));
} finally { await browser.close(); }
