import { expect, test } from '@playwright/test';
import { BUILD_SECTIONS } from '../src/data/build';
import { SHOT_CAPTURE, type ShotId } from '../src/data/shotCapture';
import { expectShotImage } from './shotImage';

const URL = '/kira-space/build/';
const ALL: readonly ShotId[] = BUILD_SECTIONS.flatMap((s) => s.shots);

test.beforeEach(async ({ page }) => {
  await page.goto(URL);
});

test('five sections in order with name, anchor, pitch and 4 to 6 figures', async ({ page }) => {
  const sections = page.locator('main section[id]');
  await expect(sections).toHaveCount(5);
  for (const [i, s] of BUILD_SECTIONS.entries()) {
    const section = sections.nth(i);
    await expect(section).toHaveAttribute('id', s.anchor);
    await expect(section.locator('h2')).toHaveText(s.name);
    await expect(section.locator('p').nth(1)).toContainText(s.pitch);
    const figures = await section.locator('figure').count();
    expect(figures).toBeGreaterThanOrEqual(4);
    expect(figures).toBeLessThanOrEqual(6);
    await expect(section.locator('figcaption').first()).not.toBeEmpty();
  }
  expect(await page.locator('main section[id] figure').count()).toBe(ALL.length);
  expect(ALL.length).toBeGreaterThanOrEqual(20);
  expect(ALL.length).toBeLessThanOrEqual(30);
});

test('the step nav lists five links to the sections', async ({ page }) => {
  const links = page.locator('nav[aria-label="Steps"] a');
  await expect(links).toHaveCount(5);
  for (const [i, s] of BUILD_SECTIONS.entries())
    await expect(links.nth(i)).toHaveAttribute('href', `#${s.anchor}`);
});

test('the page ships no carousel and no script', async ({ page }) => {
  await expect(page.locator('[data-carousel]')).toHaveCount(0);
  await expect(page.locator('main script')).toHaveCount(0);
});

test('every Build shot shows the whole image', async ({ page }) => {
  for (const id of ALL) await expectShotImage(page, id);
  for (const s of BUILD_SECTIONS) expect(SHOT_CAPTURE[s.shots[0] as ShotId].kind).toBe('window');
});
