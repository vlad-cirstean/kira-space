import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { dirname, isAbsolute, join, resolve } from 'node:path';
import { expect, type Page } from '@playwright/test';
import sharp from 'sharp';

// P280: presentation screenshots. Not a test helper: nothing here asserts app behaviour.

const ROOT_DIR = resolve(__dirname, '../../../../..');
const OUT_DIR = join(ROOT_DIR, 'screenshots');

/** The apps' minimum window (`minWindowWidth`/`minWindowHeight`, internal/shell/window.go). */
export const WINDOW = { width: 1024, height: 640 } as const;
export const SCALE = 2;
/** macOS 26 window frame, CSS px rendered at `SCALE`. Provisional until applied from a macOS 26
 *  reference screenshot (P549 step 7). The button art itself is `assets/traffic-lights.svg`. */
export const FRAME = {
  radius: 16,
  /** Transparent canvas around the window; holds the baked shadow. */
  margin: { top: 32, right: 56, bottom: 80, left: 56 },
  /** Traffic-light SVG box inside the window: first centre at 20,17. */
  lights: { x: 14, y: 11, width: 52, height: 12 },
  rim: 'inset 0 0 0 1px rgb(255 255 255 / 0.1)',
  outline: '0 0 0 0.5px rgb(0 0 0 / 0.5)',
  shadow: '0 18px 48px rgb(0 0 0 / 0.45)',
} as const;

// Fontsource packages live in the site workspace, so resolve from there.
const siteRequire = createRequire(join(ROOT_DIR, 'site', 'package.json'));

async function fontFile(pkg: string, file: string): Promise<string> {
  const dir = resolve(siteRequire.resolve(`${pkg}/package.json`), '..', 'files');
  return (await readFile(join(dir, file))).toString('base64');
}

/** Pins the UI font to Inter and the data font to JetBrains Mono before any app script runs. The
 *  apps use `-apple-system` and Menlo, which Linux lacks. */
export async function installCaptureFonts(page: Page): Promise<void> {
  const faces = [
    {
      family: 'Inter',
      weight: '100 900',
      data: await fontFile('@fontsource-variable/inter', 'inter-latin-wght-normal.woff2'),
    },
    {
      family: 'JetBrains Mono',
      weight: '400',
      data: await fontFile('@fontsource/jetbrains-mono', 'jetbrains-mono-latin-400-normal.woff2'),
    },
    {
      family: 'JetBrains Mono',
      weight: '700',
      data: await fontFile('@fontsource/jetbrains-mono', 'jetbrains-mono-latin-700-normal.woff2'),
    },
  ];
  await page.addInitScript((list) => {
    for (const f of list) {
      const face = new FontFace(f.family, `url(data:font/woff2;base64,${f.data})`, {
        weight: f.weight,
      });
      document.fonts.add(face);
      void face.load();
    }
    const apply = () => {
      const style = document.createElement('style');
      // :root:root outranks tokens.css's plain :root, which loads after this script.
      style.textContent =
        ":root:root{--kira-font-ui:'Inter',sans-serif;--kira-font-family:'JetBrains Mono',monospace}";
      (document.head ?? document.documentElement).appendChild(style);
    };
    if (document.documentElement) {
      apply();
    } else {
      new MutationObserver((_records, observer) => {
        if (!document.documentElement) return;
        observer.disconnect();
        apply();
      }).observe(document, { childList: true });
    }
  }, faces);
}

/** Pinned wall clock for every shot: Tue 2026-09-22, the day Kira Space's committed ADE fixtures
 *  are anchored to (`FIXED_NOW` in apps/kira-space/tests/ui/support/adeV2.ts). */
export const CAPTURE_NOW = 1_790_070_000_000;

/** The `relaunch()` options every shot shares. */
export const CAPTURE_LAUNCH = {
  viewport: WINDOW,
  deviceScaleFactor: SCALE,
  locale: 'en-US',
  timezoneId: 'UTC',
  clockTime: CAPTURE_NOW,
  prepare: installCaptureFonts,
} as const;

export interface ShotOptions {
  readonly id: string;
  /** The `consoleErrors` fixture of the run. */
  readonly consoleErrors: readonly string[];
}

async function frame(page: Page, raw: Buffer): Promise<Buffer> {
  const lights = await readFile(join(OUT_DIR, 'assets', 'traffic-lights.svg'));
  const browser = page.context().browser();
  if (!browser) throw new Error('captureShot needs a page from browser.newPage');
  const { margin, radius, lights: box } = FRAME;
  const canvas = {
    width: WINDOW.width + margin.left + margin.right,
    height: WINDOW.height + margin.top + margin.bottom,
  };
  // A page made by browser.newPage owns its context, so framing needs a fresh one at the same scale.
  const context = await browser.newContext({ viewport: canvas, deviceScaleFactor: SCALE });
  const shot = await context.newPage();
  try {
    await shot.setContent(`<!doctype html>
<html><body style="margin:0;background:transparent">
<div id="canvas" style="box-sizing:border-box;width:${canvas.width}px;height:${canvas.height}px;padding:${margin.top}px ${margin.right}px ${margin.bottom}px ${margin.left}px">
<div id="frame" style="position:relative;width:${WINDOW.width}px;height:${WINDOW.height}px;border-radius:${radius}px;overflow:hidden;box-shadow:${FRAME.outline}, ${FRAME.shadow}">
<img id="raw" style="display:block;width:100%;height:100%" src="data:image/png;base64,${raw.toString('base64')}">
<div style="position:absolute;inset:0;border-radius:${radius}px;box-shadow:${FRAME.rim}"></div>
<img id="lights" style="position:absolute;left:${box.x}px;top:${box.y}px;width:${box.width}px;height:${box.height}px" src="data:image/svg+xml;base64,${lights.toString('base64')}">
</div></div></body></html>`);
    await shot.evaluate(() => Promise.all([...document.images].map((img) => img.decode())));
    // Screenshot the canvas, not #frame: an element shot clips to the box and drops the shadow.
    return await shot
      .locator('#canvas')
      .screenshot({ omitBackground: true, animations: 'disabled' });
  } finally {
    await context.close();
  }
}

/** Lossless WebP of a PNG buffer. Deterministic, so reruns leave git clean. */
export function encodeWebp(png: Buffer): Promise<Buffer> {
  return sharp(png).webp({ lossless: true, effort: 6 }).toBuffer();
}

/** Encodes and writes `screenshots/<name>.webp`; `name` is a relative path without extension. */
export async function saveShot(name: string, png: Buffer): Promise<string> {
  if (isAbsolute(name) || name.split('/').includes('..') || /\.[a-z0-9]+$/i.test(name)) {
    throw new Error(`saveShot name must be a relative path without extension: ${name}`);
  }
  const file = join(OUT_DIR, `${name}.webp`);
  await mkdir(dirname(file), { recursive: true });
  await writeFile(file, await encodeWebp(png));
  return file;
}

/** Readiness steps every capture needs before the first screenshot. */
export async function prepareCapture(page: Page, consoleErrors: readonly string[]): Promise<void> {
  await page.mouse.move(0, 0);
  // Monaco fades its scrollbars on a JS timer, which `animations: 'disabled'` cannot freeze.
  await page.addStyleTag({ content: '.monaco-editor .scrollbar { opacity: 0 !important; }' });
  await page.evaluate(() => document.fonts.ready);
  expect(await page.evaluate(() => document.fonts.check('13px Inter'))).toBe(true);
  expect(consoleErrors).toEqual([]);
}

/** Writes `screenshots/framed/<id>.webp`; the raw PNG stays in memory. Call once the view shows its key content: the caller asserts that first. */
export async function captureShot(page: Page, options: ShotOptions): Promise<void> {
  const { id, consoleErrors } = options;
  await prepareCapture(page, consoleErrors);

  const capture = { animations: 'disabled', caret: 'hide', scale: 'device' } as const;
  const raw = await page.screenshot(capture);

  await saveShot(join('framed', id), await frame(page, raw));
}
