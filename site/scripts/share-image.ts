#!/usr/bin/env bun
/** Renders the share image (og:image, twitter:image): site/src/assets/share/kira-share.webp. Rerun after a headline or icon change. */
import { writeFileSync } from 'node:fs';
import { chromium } from '@playwright/test';
import sharp from 'sharp';
import { HASH_FILE, OUT, shareImageHash, shareImageHtml } from './share-image-input';

const html = shareImageHtml();

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
writeFileSync(HASH_FILE, `${shareImageHash()}\n`);
process.stderr.write(`share-image: wrote ${OUT}\n`);
