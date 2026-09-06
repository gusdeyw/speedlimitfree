import { chromium } from '../frontend/node_modules/@playwright/test/index.mjs';
import { readFile } from 'node:fs/promises';
const browser = await chromium.launch({ headless: true });
try {
  const page = await browser.newPage({ viewport: { width: 256, height: 256 }, deviceScaleFactor: 1 });
  await page.setContent(`<style>body{margin:0;background:transparent}svg{display:block}</style>${await readFile(new URL('../build/appicon.svg', import.meta.url), 'utf8')}`);
  await page.screenshot({ path: new URL('../build/appicon.png', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1'), omitBackground: true });
} finally { await browser.close(); }
