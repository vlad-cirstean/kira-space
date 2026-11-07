import { readdirSync, readFileSync } from 'node:fs';
import { join, relative } from 'node:path';
import { expect, type Page, test } from '@playwright/test';
import type { ShotId } from '../src/data/shotCapture';
import { expectShotImage } from './shotImage';

const BASE = '/kira-space/';
const DOCS_DIR = join(import.meta.dirname, '..', 'src', 'content', 'docs', 'docs');

interface DocPage {
  readonly url: string;
  readonly title: string;
  readonly file: string;
  readonly path: string;
}

function walk(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((e) =>
    e.isDirectory() ? walk(join(dir, e.name)) : [join(dir, e.name)],
  );
}

function pageOf(file: string): DocPage {
  const rel = relative(DOCS_DIR, file).replace(/\.mdx?$/, '');
  const slug = rel === 'index' ? '' : `${rel}/`;
  const title = /^title: (.+)$/m.exec(readFileSync(file, 'utf8'))?.[1];
  if (!title) throw new Error(`No title in ${file}`);
  return { url: `${BASE}docs/${slug}`, title, file: rel, path: file };
}

const PAGES = walk(DOCS_DIR)
  .filter((f) => /\.mdx?$/.test(f))
  .sort()
  .map(pageOf);

const SHOT_PAGES: Record<string, readonly ShotId[]> = {
  'studio/data-grid': ['studio-data-grid', 'studio-grid-row-menu'],
  'studio/sql-console': ['studio-sql-console'],
  'studio/database-mcp': ['build-db-approval', 'build-db-audit'],
  'studio/api-client': ['studio-api'],
  'space/git': ['space-git-graph', 'space-commit-detail'],
  'space/agents': [
    'space-agents-board',
    'build-branches-panel',
    'build-handover-dialog',
    'build-review-window',
  ],
  'space/workflows': ['build-start-workflow', 'build-start-step'],
  'space/proof': [
    'build-proof-window',
    'build-proof-chart',
    'build-proof-diagram',
    'build-proof-compare',
    'build-proof-chain',
  ],
  'space/memory': ['space-memory'],
};

const visited = new Set<string>();

function watchErrors(page: Page): string[] {
  const errors: string[] = [];
  page.on('console', (msg) => {
    if (msg.type() === 'error') errors.push(msg.text());
  });
  page.on('pageerror', (err) => errors.push(err.message));
  return errors;
}

test('the page list is not empty', () => {
  expect(PAGES.length).toBeGreaterThanOrEqual(25);
});

test('the Develop section is gone', async ({ request }) => {
  const res = await request.get(`${BASE}docs/develop/`);
  expect(res.status()).toBe(404);
  for (const doc of PAGES) {
    expect(readFileSync(doc.path, 'utf8'), doc.file).not.toContain('develop/');
  }
});

for (const doc of PAGES) {
  test.describe(doc.file, () => {
    test('loads without errors, scroll or broken assets', async ({ page, baseURL }) => {
      const errors = watchErrors(page);
      const response = await page.goto(doc.url);
      expect(response?.status()).toBe(200);
      await expect(page.locator('h1')).toHaveText(doc.title);

      const metrics = await page.evaluate(() => ({
        scrollWidth: document.documentElement.scrollWidth,
        innerWidth: window.innerWidth,
        html: getComputedStyle(document.documentElement).overflowX,
        body: getComputedStyle(document.body).overflowX,
      }));
      expect(metrics.scrollWidth).toBeLessThanOrEqual(metrics.innerWidth);
      expect(['hidden', 'clip']).not.toContain(metrics.html);
      expect(['hidden', 'clip']).not.toContain(metrics.body);

      const width = page.viewportSize()?.width ?? 0;
      const images = page.locator('main img');
      for (let i = 0; i < (await images.count()); i++) {
        const img = images.nth(i);
        await img.scrollIntoViewIfNeeded();
        await expect
          .poll(() => img.evaluate((el: HTMLImageElement) => el.complete && el.naturalWidth > 0))
          .toBe(true);
        const box = await img.boundingBox();
        expect(box).not.toBeNull();
        expect((box?.x ?? 0) + (box?.width ?? 0)).toBeLessThanOrEqual(width + 0.5);
      }

      const urls = await page.evaluate(() => {
        const out = new Set<string>();
        for (const el of document.querySelectorAll('a[href], link[href]')) {
          out.add(new URL(el.getAttribute('href') ?? '', document.baseURI).href);
        }
        for (const el of document.querySelectorAll('img[src], script[src]')) {
          out.add(new URL(el.getAttribute('src') ?? '', document.baseURI).href);
        }
        return [...out];
      });
      for (const href of urls) {
        const url = new URL(href);
        url.hash = '';
        if (url.origin !== baseURL || !url.pathname.startsWith(BASE) || visited.has(url.href)) {
          continue;
        }
        visited.add(url.href);
        const res = await page.request.get(url.href);
        expect(res.status(), url.href).toBe(200);
      }

      await page.waitForLoadState('networkidle');
      expect(errors).toEqual([]);
    });

    test('is readable and navigable at this width', async ({ page }, info) => {
      await page.goto(doc.url);
      if (info.project.name === 'phone') {
        const small = await page.evaluate(() =>
          [...document.querySelectorAll('.sl-markdown-content :is(p, li)')]
            .filter((el) => el.getClientRects().length > 0)
            .map((el) => ({
              text: (el.textContent ?? '').slice(0, 40),
              px: parseFloat(getComputedStyle(el).fontSize),
            }))
            .filter((item) => item.px < 16),
        );
        expect(small).toEqual([]);

        await page.locator('button.sl-menu-button').click();
        const tight = await page.evaluate(() =>
          [...document.querySelectorAll('#starlight__sidebar a, header button')]
            .filter((el) => el.getClientRects().length > 0)
            .map((el) => ({
              text: (el.textContent ?? el.getAttribute('aria-label') ?? '').trim().slice(0, 40),
              h: el.getBoundingClientRect().height,
            }))
            .filter((item) => item.h < 44),
        );
        expect(tight).toEqual([]);
      } else {
        await expect(page.locator('#starlight__sidebar')).toBeVisible();
        await expect(page.locator('#starlight__sidebar a[aria-current="page"]')).toHaveCount(1);
      }
    });

    for (const shot of SHOT_PAGES[doc.file] ?? []) {
      test(`shot ${shot} uses the right image for the viewport`, async ({ page }, info) => {
        await page.goto(doc.url);
        await expectShotImage(page, shot, info.project.name === 'phone');
      });
    }
  });
}

test('search finds a tool and opens its page', async ({ page }) => {
  await page.goto(`${BASE}docs/`);
  await page.locator('site-search button').first().click();
  await page
    .locator('input[type="search"], .pagefind-ui__search-input')
    .first()
    .fill('store_memory');
  const hit = page.locator('dialog a[href*="docs/reference/mcp-tools"]').first();
  await expect(hit).toBeVisible();
  await hit.click();
  await expect(page).toHaveURL(/docs\/reference\/mcp-tools\//);
  await expect(page.locator('h1')).toHaveText('MCP tools');
});

test('phone menu opens a page', async ({ page }, info) => {
  test.skip(info.project.name !== 'phone', 'phone only');
  await page.goto(`${BASE}docs/`);
  await page.locator('button.sl-menu-button').click();
  await page.locator('#starlight__sidebar').getByRole('link', { name: 'SQL console' }).click();
  await expect(page).toHaveURL(/docs\/studio\/sql-console\//);
  await expect(page.locator('h1')).toHaveText('SQL console');
});

test('the landing Docs link reaches the docs and the site title returns', async ({ page }) => {
  await page.goto(BASE);
  const docs = page.locator('header').getByRole('link', { name: 'Docs', exact: true });
  const href = await docs.getAttribute('href');
  expect(href).toBe(`${BASE}docs/`);
  const res = await page.request.get(new URL(href ?? '', page.url()).href);
  expect(res.status()).toBe(200);
  await page.goto(`${BASE}docs/`);
  await expect(page.locator('a.site-title')).toHaveAttribute('href', BASE);
});

test.describe('dark only', () => {
  test.use({ colorScheme: 'light' });
  test('light colour scheme still renders the dark theme', async ({ page }) => {
    await page.goto(`${BASE}docs/`);
    const theme = await page.locator('html').getAttribute('data-theme');
    expect(theme).not.toBe('light');
  });
});
