import { expect, type Page, test } from '@playwright/test';
import { BUILD_SECTIONS } from '../src/data/build';
import { SHOT_CAPTURE, type ShotId } from '../src/data/shotCapture';
import { expectShotImage } from './shotImage';

const URL = '/kira-studio/build/';
const ALL: readonly ShotId[] = BUILD_SECTIONS.flatMap((s) => s.shots);

const dots = (page: Page) => page.locator('[data-carousel] [data-dot]');
const active = (page: Page) =>
  dots(page).evaluateAll((els) => els.findIndex((e) => e.getAttribute('aria-current') === 'true'));

// The observer reports on frames the fake clock drives, so tick it in small steps while polling.
const paused = async (page: Page, tick: number) => {
  await page.clock.runFor(tick);
  return (await page.locator('[data-carousel]').getAttribute('data-paused')) ?? '';
};

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

test('five slides name their step and a slide link lands on its section', async ({ page }) => {
  const slides = page.locator('[data-carousel] [data-slide]');
  await expect(slides).toHaveCount(5);
  for (const [i, s] of BUILD_SECTIONS.entries()) {
    await expect(slides.nth(i)).toHaveAttribute(
      'aria-label',
      new RegExp(`^${s.step} of 5: ${s.name}$`),
    );
    await expect(slides.nth(i).locator('a')).toHaveAttribute('href', `#${s.anchor}`);
  }
  await page.locator('[data-carousel] [data-dot]').nth(2).click();
  await expect.poll(() => active(page)).toBe(2);
  await slides.nth(2).locator('a').click();
  await expect(page).toHaveURL(/#branches-handover$/);
  await expect(page.locator('#branches-handover')).toBeInViewport();
});

test('arrows, keys and dots move the active slide and wrap', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  expect(await active(page)).toBe(0);
  const next = page.getByRole('button', { name: 'Next step' });
  const prev = page.getByRole('button', { name: 'Previous step' });
  await next.click();
  await expect.poll(() => active(page)).toBe(1);
  await prev.click();
  await expect.poll(() => active(page)).toBe(0);
  await prev.click();
  await expect.poll(() => active(page)).toBe(4);
  await next.click();
  await expect.poll(() => active(page)).toBe(0);
  await next.focus();
  await page.keyboard.press('ArrowRight');
  await expect.poll(() => active(page)).toBe(1);
  await page.keyboard.press('ArrowLeft');
  await expect.poll(() => active(page)).toBe(0);
});

test('the carousel has no play or pause control', async ({ page }) => {
  await expect(page.getByRole('button', { name: /play|pause/i })).toHaveCount(0);
});

test.describe('autoplay', () => {
  test.beforeEach(async ({ page }) => {
    await page.clock.install();
    await page.goto(URL);
    await page.locator('[data-carousel]').scrollIntoViewIfNeeded();
  });

  test('advances after 6 s', async ({ page }) => {
    await expect.poll(() => active(page)).toBe(0);
    await page.clock.runFor(6100);
    await expect.poll(() => active(page)).toBe(1);
  });

  test('holds while hovered, then resumes', async ({ page }) => {
    await page.locator('[data-carousel] [data-track]').hover();
    await page.clock.runFor(13_000);
    expect(await active(page)).toBe(0);
    await page.mouse.move(0, 0);
    await page.clock.runFor(6100);
    await expect.poll(() => active(page)).toBe(1);
  });

  test('holds while focus is inside, then resumes', async ({ page }) => {
    await page.getByRole('button', { name: 'Next step' }).focus();
    await page.clock.runFor(13_000);
    expect(await active(page)).toBe(0);
    await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
    await page.clock.runFor(6100);
    await expect.poll(() => active(page)).toBe(1);
  });

  test('holds while scrolled off screen, then resumes', async ({ page }) => {
    await page.evaluate(() =>
      window.scrollTo({ top: document.documentElement.scrollHeight, behavior: 'instant' }),
    );
    await expect.poll(() => paused(page, 100)).toContain('offscreen');
    await page.clock.runFor(13_000);
    expect(await active(page)).toBe(0);
    await page.locator('[data-carousel]').scrollIntoViewIfNeeded();
    await expect.poll(() => paused(page, 100)).not.toContain('offscreen');
    await page.clock.runFor(6100);
    await expect.poll(() => active(page)).toBe(1);
  });

  test('holds while the tab is hidden, then resumes', async ({ page }) => {
    await page.evaluate(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'hidden', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    await page.clock.runFor(13_000);
    expect(await active(page)).toBe(0);
    await page.evaluate(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    await page.clock.runFor(6100);
    await expect.poll(() => active(page)).toBe(1);
  });
});

test('reduced motion never autoplays while the arrows still work', async ({ page }) => {
  await page.clock.install();
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto(URL);
  await page.clock.runFor(20_000);
  expect(await active(page)).toBe(0);
  await page.getByRole('button', { name: 'Next step' }).click();
  await expect.poll(() => active(page)).toBe(1);
});

test('every Build shot serves the right image for its kind', async ({ page }, info) => {
  const phone = info.project.name === 'phone';
  for (const id of ALL) await expectShotImage(page, id, phone);
  for (const s of BUILD_SECTIONS) expect(SHOT_CAPTURE[s.shots[0] as ShotId].kind).toBe('window');
});
