import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect, type Page, test } from '@playwright/test';
import { BUILD_SECTIONS } from '../src/data/build';
import type { ShotId } from '../src/data/shotCapture';
import { expectShotImage } from './shotImage';

const BASE = '/kira-space/';
interface Route {
  readonly path: string;
  readonly name: string;
  readonly hero: ShotId;
  readonly shots: readonly ShotId[];
}

const ROUTES: readonly Route[] = [
  {
    path: '',
    name: 'Kira Space',
    hero: 'space-agents-board',
    shots: [
      'space-agents-board',
      'space-git-graph',
      'space-memory',
      'studio-sql-console',
      'studio-api',
    ],
  },
  {
    path: 'build/',
    name: 'Build with Kira',
    hero: 'build-start-board',
    shots: BUILD_SECTIONS.flatMap((section) => section.shots),
  },
];

async function openPage(page: Page, route: Route): Promise<string[]> {
  const errors: string[] = [];
  page.on('console', (msg) => {
    if (msg.type() === 'error') errors.push(msg.text());
  });
  page.on('pageerror', (err) => errors.push(err.message));
  await page.goto(`${BASE}${route.path}`);
  return errors;
}

for (const route of ROUTES) {
  test.describe(route.path === '' ? 'landing' : route.path, () => {
    test('no horizontal scroll and no hidden overflow', async ({ page }) => {
      await openPage(page, route);
      const metrics = await page.evaluate(() => ({
        scrollWidth: document.documentElement.scrollWidth,
        innerWidth: window.innerWidth,
        html: getComputedStyle(document.documentElement).overflowX,
        body: getComputedStyle(document.body).overflowX,
      }));
      expect(metrics.scrollWidth).toBeLessThanOrEqual(metrics.innerWidth);
      expect(['hidden', 'clip']).not.toContain(metrics.html);
      expect(['hidden', 'clip']).not.toContain(metrics.body);
    });

    test('every image loads and fits the viewport', async ({ page }) => {
      await openPage(page, route);
      const images = page.locator('img');
      const count = await images.count();
      expect(count).toBeGreaterThanOrEqual(route.shots.length);
      const width = page.viewportSize()?.width ?? 0;
      for (let i = 0; i < count; i++) {
        const img = images.nth(i);
        await img.scrollIntoViewIfNeeded();
        await expect
          .poll(() => img.evaluate((el: HTMLImageElement) => el.complete && el.naturalWidth > 0))
          .toBe(true);
        const box = await img.boundingBox();
        expect(box).not.toBeNull();
        expect(box?.x ?? -1).toBeGreaterThanOrEqual(0);
        expect((box?.x ?? 0) + (box?.width ?? 0)).toBeLessThanOrEqual(width + 0.5);
      }
    });

    test('every same-origin link and asset answers 200', async ({ page, baseURL }) => {
      await openPage(page, route);
      const urls = await page.evaluate(() => {
        const out = new Set<string>();
        const add = (value: string | null) => {
          if (!value) return;
          for (const part of value.split(',')) {
            const candidate = part.trim().split(/\s+/)[0];
            if (candidate) out.add(new URL(candidate, document.baseURI).href);
          }
        };
        for (const el of document.querySelectorAll('a[href], link[href]')) {
          add(el.getAttribute('href'));
        }
        for (const el of document.querySelectorAll('img[src], script[src]')) {
          add(el.getAttribute('src'));
        }
        for (const el of document.querySelectorAll('source[srcset]')) {
          add(el.getAttribute('srcset'));
        }
        return [...out];
      });
      const local = urls
        .map((href) => new URL(href))
        .filter((url) => url.origin === baseURL && url.pathname.startsWith(BASE));
      expect(local.length).toBeGreaterThan(route.shots.length);
      for (const url of local) {
        url.hash = '';
        const response = await page.request.get(url.href);
        expect(response.status(), url.href).toBe(200);
      }
    });

    test('no console errors while scrolling the page', async ({ page }) => {
      const errors = await openPage(page, route);
      await page.evaluate(async () => {
        for (let y = 0; y < document.documentElement.scrollHeight; y += window.innerHeight) {
          window.scrollTo(0, y);
          await new Promise((resolve) => requestAnimationFrame(() => resolve(null)));
        }
      });
      await page.waitForLoadState('networkidle');
      expect(errors).toEqual([]);
    });

    test('shots use the crop on a phone and the framed window on desktop', async ({
      page,
    }, info) => {
      await openPage(page, route);
      const phone = info.project.name === 'phone';
      if (phone) expect(page.viewportSize()?.width).toBe(375);
      for (const id of route.shots) await expectShotImage(page, id, phone);
    });

    test('body text is readable and tap targets are large enough', async ({ page }, info) => {
      await openPage(page, route);
      if (info.project.name === 'phone') {
        const small = await page.evaluate(() =>
          [...document.querySelectorAll('p, li')]
            .filter((el) => el.getClientRects().length > 0)
            .map((el) => ({
              text: (el.textContent ?? '').slice(0, 40),
              px: parseFloat(getComputedStyle(el).fontSize),
            }))
            .filter((item) => item.px < 16),
        );
        expect(small).toEqual([]);
      }
      const tight = await page.evaluate(() =>
        [...document.querySelectorAll('a')]
          .filter((el) => !el.closest('p') && el.getClientRects().length > 0)
          .map((el) => ({
            text: (el.textContent ?? el.getAttribute('aria-label') ?? '').trim(),
            h: el.getBoundingClientRect().height,
          }))
          .filter((item) => item.h < 44),
      );
      expect(tight).toEqual([]);
    });

    test('footer credits Codicons', async ({ page }) => {
      await openPage(page, route);
      await expect(page.locator('footer a[href*="vscode-codicons"]')).toHaveCount(1);
      await expect(
        page.locator('footer a[href*="creativecommons.org/licenses/by/4.0"]'),
      ).toHaveCount(1);
    });

    test('head names this tab and its hero image', async ({ page }) => {
      await openPage(page, route);
      await expect(page.locator('h1')).toHaveText(route.name);
      await expect(page).toHaveTitle(new RegExp(route.name));
      const og = await page.locator('meta[property="og:image"]').getAttribute('content');
      expect(og).toContain(route.hero);
    });

    test('every icon is hidden from assistive tech or labelled', async ({ page }) => {
      await openPage(page, route);
      const bare = await page.evaluate(() =>
        [...document.querySelectorAll('main svg, footer svg')]
          .filter((el) => !el.closest('[aria-hidden="true"], [aria-label]'))
          .map((el) => el.parentElement?.className ?? ''),
      );
      expect(bare).toEqual([]);
    });
  });
}

test('the retired /space/ URL redirects to the landing page', async ({ page }) => {
  await page.goto(`${BASE}space/`);
  await expect(page).toHaveURL(BASE);
  await expect(page.locator('h1')).toHaveText('Kira Space');
});

test('the landing page has no app tab bar', async ({ page }) => {
  await page.goto(BASE);
  await expect(page.locator('nav[aria-label="Apps"]')).toHaveCount(0);
});

test('the hero shot is published at the stable URL the README links to', async ({ request }) => {
  for (const route of ROUTES) {
    const res = await request.get(`${BASE}shots/${route.hero}.webp`);
    expect(res.status()).toBe(200);
    expect(res.headers()['content-type']).toContain('image/webp');
  }
});

for (const path of ['', 'build/']) {
  test(`beta notice is first in the body of /${path}`, async ({ page }) => {
    await page.goto(`${BASE}${path}`);
    const first = page.locator('body > *').first();
    await expect(first).toHaveAttribute('data-beta-notice', '');
    await expect(first).toContainText('Beta, not production ready, not notarized.');
    await expect(first.locator('a')).toHaveAttribute(
      'href',
      new RegExp(path === '' ? '^#install$' : '/kira-space/#install$'),
    );
    await expect(first).not.toContainText(/keychain|folder|macos/i);
  });
}

test('install holds the Keychain image and the folder text', async ({ page }) => {
  await page.goto(BASE);
  const install = page.locator('#install');
  await expect(install).toContainText('its own key');
  await expect(install).toContainText('Always Allow');
  await expect(install).toContainText('folders that hold your repositories');
  await expect(install.locator('img')).toHaveCount(1);
  const keychain = install.locator(
    'img[alt="macOS Keychain dialog asking to let Kira Space use its own key."]',
  );
  await expect(keychain).toHaveCount(1);
  await keychain.scrollIntoViewIfNeeded();
  await expect.poll(() => keychain.evaluate((el: HTMLImageElement) => el.naturalWidth)).toBe(868);
  await expect(install.locator('img[alt*="access a folder"]')).toHaveCount(0);
  await expect(install).not.toContainText('Screenshot pending');
});

test('the README opens with the beta notice and the install link', () => {
  const first = readFileSync(join(import.meta.dirname, '..', '..', 'README.md'), 'utf8')
    .split('\n')
    .find((line) => line.trim() !== '');
  expect(first).toContain('Beta, not production ready, not notarized.');
  expect(first).toContain('https://vlad-cirstean.github.io/kira-space/#install');
});
