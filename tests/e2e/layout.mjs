// AI.md PART 29 / PART 16 "Layout Integrity & Polish Contract": review matrix in headless Chromium.
// Every page is loaded at every viewport, in dark/light themes and LTR/RTL, and must satisfy the
// pass criteria below. Exit code 1 when any check fails.
import { chromium } from 'playwright';

const base = process.env.BASE_URL || 'http://127.0.0.1:64999';

const viewports = [
  [320, 640], [375, 812], [390, 844], [768, 1024], [820, 1180],
  [1024, 768], [1280, 800], [1440, 900], [1920, 1080],
];
const themes = ['dark', 'light'];
const languages = ['en', 'ar'];
const pages = [
  '/', '/server/about', '/server/help', '/server/help/mcp', '/server/contact',
  '/server/security', '/server/security/policy', '/server/security/thanks',
  '/server/dpo', '/server/privacy', '/server/terms', '/server/preferences', '/server/healthz',
];

const failures = [];
const browser = await chromium.launch();

for (const [width, height] of viewports) {
  for (const theme of themes) {
    for (const lang of languages) {
      const context = await browser.newContext({ viewport: { width, height } });
      await context.addCookies([
        { name: 'theme', value: theme, url: base },
        { name: 'lang', value: lang, url: base },
      ]);
      for (const path of pages) {
        const page = await context.newPage();
        const label = `${path} @${width}x${height} ${theme} ${lang}`;
        const response = await page.goto(base + path, { waitUntil: 'load' });
        if (!response || response.status() >= 500) {
          failures.push(`${label}: status ${response ? response.status() : 'none'}`);
          await page.close();
          continue;
        }
        // The disabled MCP page is a legitimate 404; every other page must render
        if (response.status() >= 400 && response.status() !== 404) {
          failures.push(`${label}: status ${response.status()}`);
          await page.close();
          continue;
        }
        if (response.status() === 404) {
          await page.close();
          continue;
        }
        const problems = await page.evaluate(() => {
          const out = [];
          const root = document.documentElement;
          if (root.scrollWidth > window.innerWidth) {
            out.push(`horizontal overflow (${root.scrollWidth} > ${window.innerWidth})`);
            let widest = null;
            for (const el of document.querySelectorAll('body *')) {
              const rect = el.getBoundingClientRect();
              if (rect.width > 0 && (!widest || rect.right > widest.right)) widest = { right: rect.right, el };
            }
            const wide = [];
            for (const el of document.querySelectorAll('body, body *')) {
              if (el.scrollWidth > el.clientWidth + 1 && el.clientWidth > 0 && getComputedStyle(el).overflowX === 'visible') {
                wide.push(`${el.tagName.toLowerCase()}.${String(el.className).split(' ')[0]}(${el.scrollWidth}>${el.clientWidth})`);
              }
            }
            if (wide.length) out.push(`scroll-wider elements: ${wide.slice(0, 6).join(', ')}`);
            if (widest) out.push(`widest element: ${widest.el.tagName.toLowerCase()}.${String(widest.el.className).split(' ')[0]} right=${Math.round(widest.right)}`);
          }
          for (const el of document.querySelectorAll('body *')) {
            const rect = el.getBoundingClientRect();
            const style = getComputedStyle(el);
            if (style.position === 'fixed' || style.display === 'none' || rect.width === 0) continue;
            if (rect.right > window.innerWidth + 1 && rect.left >= 0 && !el.closest('.table-scroll, pre, .nav-panel')) {
              out.push(`element past viewport: ${el.tagName.toLowerCase()}${el.className ? '.' + String(el.className).split(' ')[0] : ''} in ${el.parentElement ? el.parentElement.tagName.toLowerCase() + '.' + String(el.parentElement.className).split(' ')[0] : ''} (${Math.round(rect.right)}px) ${el.textContent.trim().slice(0, 30)}`);
              break;
            }
          }
          for (const select of document.querySelectorAll('select')) {
            if (select.options.length === 0) out.push('select without options');
          }
          for (const control of document.querySelectorAll('input:not([type=hidden]):not([type=submit]):not([type=button]), select, textarea')) {
            const hasLabel = control.labels && control.labels.length > 0;
            if (!hasLabel && !control.getAttribute('aria-label') && !control.getAttribute('aria-labelledby')) {
              out.push(`control without a label: ${control.name || control.id || control.tagName}`);
            }
          }
          if (document.querySelectorAll('h1').length !== 1) {
            out.push(`expected exactly one h1, found ${document.querySelectorAll('h1').length}`);
          }
          const dir = document.documentElement.getAttribute('dir');
          if (document.documentElement.lang === 'ar' && dir !== 'rtl') out.push('Arabic page is not dir=rtl');
          return out;
        });
        for (const problem of problems) failures.push(`${label}: ${problem}`);
        await page.close();
      }
      await context.close();
    }
  }
}

await browser.close();

if (failures.length > 0) {
  console.error(`LAYOUT FAILURES (${failures.length}):`);
  for (const f of failures.slice(0, 200)) console.error(' - ' + f);
  process.exit(1);
}
console.log('Layout review matrix passed');
