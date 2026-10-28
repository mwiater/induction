// Run with Playwright installed, or set INDUCTION_PLAYWRIGHT_PACKAGE to its index.mjs.
// Optional: INDUCTION_CHROME_PATH selects an existing Chromium binary.
import assert from 'node:assert/strict';
import { mkdir, realpath } from 'node:fs/promises';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

const playwright = await import(process.env.INDUCTION_PLAYWRIGHT_PACKAGE || 'playwright');
const htmlPath = await realpath(process.argv[2] || 'data/dashboard/dashboard.html');
const screenshotDirectory = process.argv[3] ? resolve(process.argv[3]) : null;
if (screenshotDirectory) await mkdir(screenshotDirectory, { recursive: true });
const browser = await playwright.chromium.launch({
  headless: true,
  ...(process.env.INDUCTION_CHROME_PATH ? { executablePath: process.env.INDUCTION_CHROME_PATH } : {}),
  args: ['--no-sandbox', '--disable-dev-shm-usage', '--disable-background-networking'],
});
const page = await browser.newPage({ viewport: { width: 390, height: 844 }, reducedMotion: 'reduce' });
const errors = [];
page.on('pageerror', error => errors.push(error.message));
page.on('console', message => { if (message.type() === 'error') errors.push(message.text()); });
const widths = [320, 360, 375, 390, 430, 600, 768, 1024, 1280, 1440, 1920];
const views = ['overview', 'performance', 'behavior', 'models', 'recommendations', 'playground', 'variables'];
try {
  await page.goto(pathToFileURL(htmlPath).href, { waitUntil: 'load' });
  for (const theme of ['dark', 'light']) {
    if (await page.locator('html').getAttribute('data-theme') !== theme) await page.locator('#themeToggle').click();
    for (const width of widths) {
      await page.setViewportSize({ width, height: width < 600 ? 844 : 900 });
      for (const view of views) {
        await page.locator(`[data-view="${view}"]`).click();
        const geometry = await page.evaluate(() => ({
          viewport: innerWidth,
          document: document.documentElement.scrollWidth,
          body: document.body.scrollWidth,
          active: document.querySelector('.dashboard-view.active')?.id,
        }));
        assert.equal(geometry.active, view, `${theme} ${width}: wrong active view`);
        assert.ok(geometry.document <= geometry.viewport + 1 && geometry.body <= geometry.viewport + 1,
          `${theme} ${width} ${view}: page overflow ${JSON.stringify(geometry)}`);
        if (screenshotDirectory) {
          await page.screenshot({ path: resolve(screenshotDirectory, `${theme}-${width}-${view}.png`), fullPage: true });
        }
      }
    }
  }
  await page.setViewportSize({ width: 390, height: 844 });
  await page.locator('[data-view="performance"]').click();
  assert.ok(await page.locator('#scenarioCold').isChecked());
  const trackWithLoad = page.locator('#timeBreakdown .time-track').filter({ has: page.locator('.load') }).first();
  if (await trackWithLoad.count()) {
    assert.ok(await trackWithLoad.locator(':scope > span').first().evaluate(element => element.classList.contains('load')));
  }
  await page.locator('#scenarioCold').uncheck();
  assert.equal(await page.locator('#timeBreakdown .time-track .load').count(), 0);
  await page.locator('#scenarioCold').check();
  await page.locator('[data-view="recommendations"]').click();
  assert.equal(await page.locator('#recommendation .rec-workload').count(), 6);
  assert.equal(await page.locator('#workload').count(), 0);
  assert.equal(await page.locator('#recommendation .rec-alternatives').count(), 0);
  assert.ok(await page.locator('#rec-context .rec-duel-lane').count() >= 2);
  for (const label of await page.locator('#recommendation .rec-duel-model').all()) {
    assert.equal(await label.innerText(), await label.getAttribute('title'));
  }
  await page.locator('[data-workload="vision"]').click();
  assert.ok(await page.locator('#rec-vision').evaluate(element => element.classList.contains('in-view')));
  await page.locator('[data-view="playground"]').click();
  for (const row of await page.locator('#playLoom .loom-row').all()) {
    assert.equal(await row.locator('.loom-label').textContent(), await row.getAttribute('data-play-model'));
  }
  await page.locator('[data-view="models"]').click();
  assert.equal(await page.locator('#radarSelectionNames strong').first().textContent(), await page.locator('#radarA').inputValue());
  await page.locator('#clearAll').click();
  assert.match(await page.locator('#filterCount').innerText(), /^0 of /);
  await page.locator('#selectAll').click();
  await page.locator('[data-view="models"]').click();
  await page.locator('#matrix [data-inspect="model"]').first().focus();
  await page.keyboard.press('Enter');
  assert.ok(await page.locator('#inspector').evaluate(element => element.classList.contains('open')));
  await page.keyboard.press('Escape');
  assert.ok(!await page.locator('#inspector').evaluate(element => element.classList.contains('open')));
  await page.locator('[data-view="variables"]').click();
  const variableCount = await page.locator('#variableCount').innerText();
  await page.locator('[data-view="overview"]').click();
  await page.locator('#clearAll').click();
  await page.locator('[data-view="variables"]').click();
  assert.equal(await page.locator('#variableCount').innerText(), variableCount);
  await page.locator('#themeToggle').click();
  const persistedTheme = await page.locator('html').getAttribute('data-theme');
  await page.reload();
  assert.equal(await page.locator('html').getAttribute('data-theme'), persistedTheme);
  assert.equal(await page.locator('.dashboard-view.active').getAttribute('id'), 'variables');
  assert.deepEqual(errors, [], 'browser console errors');
  console.log(`Dashboard browser checks passed: ${widths.length} widths, ${views.length} views, two themes, interactions.`);
} finally {
  await browser.close();
}
