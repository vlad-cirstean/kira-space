#!/usr/bin/env bun
/** Renders the share image (og:image, twitter:image): site/src/assets/share/kira-share.webp. Rerun after a headline or icon change. */
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { chromium } from '@playwright/test';
import sharp from 'sharp';
import { HERO, SITE } from '../src/data/content';

const SITE_DIR = join(import.meta.dir, '..');
const ICON = join(SITE_DIR, '..', 'apps', 'kira-space', 'build', 'appicon.png');
const OUT = join(SITE_DIR, 'src', 'assets', 'share', 'kira-share.webp');
const FONT = Bun.resolveSync(
  '@fontsource-variable/inter/files/inter-latin-wght-normal.woff2',
  SITE_DIR,
);
const b64 = (path: string) => readFileSync(path).toString('base64');

const html = `<!doctype html><html><head><meta charset="utf-8"><style>
@font-face { font-family: Inter; font-weight: 100 900; src: url(data:font/woff2;base64,${b64(FONT)}) format('woff2'); }
* { box-sizing: border-box; margin: 0; }
body {
  width: 1200px; height: 630px; padding: 72px; display: flex; align-items: center; gap: 64px;
  font-family: Inter, sans-serif; color: #fff; overflow: hidden;
  background:
    radial-gradient(circle at 0% 0%, rgba(59,155,232,0.35), transparent 55%),
    radial-gradient(circle at 100% 100%, rgba(10,95,168,0.30), transparent 55%),
    #181818;
}
img { width: 320px; height: 320px; flex: none; }
.col { max-width: 640px; }
.name { font-size: 40px; font-weight: 600; color: #9fd2f7; }
h1 { margin-top: 20px; font-size: 76px; font-weight: 800; line-height: 1.05; letter-spacing: -0.03em; }
h1 span { display: block; }
.note { margin-top: 28px; text-wrap: balance; font-size: 26px; font-weight: 500; color: #b8b8b8; }
</style></head><body>
<img src="data:image/png;base64,${b64(ICON)}" alt="">
<div class="col">
  <div class="name">${HERO.eyebrow}</div>
  <h1><span>${HERO.line1}</span><span>${HERO.line2}</span></h1>
  <div class="note">${SITE.availability}</div>
</div>
</body></html>`;

const browser = await chromium.launch();
try {
  const page = await browser.newPage({
    viewport: { width: 1200, height: 630 },
    deviceScaleFactor: 1,
  });
  await page.setContent(html);
  await page.evaluate(() => document.fonts.ready);
  const png = await page.screenshot({ type: 'png' });
  await sharp(png).webp({ lossless: true, effort: 6 }).toFile(OUT);
} finally {
  await browser.close();
}

const meta = await sharp(OUT).metadata();
if (meta.width !== 1200 || meta.height !== 630) {
  process.stderr.write(`share-image: ${OUT} is ${meta.width}x${meta.height}, expected 1200x630\n`);
  process.exit(1);
}
process.stderr.write(`share-image: wrote ${OUT}\n`);
