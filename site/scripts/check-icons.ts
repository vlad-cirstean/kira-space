#!/usr/bin/env bun
/** Checks the built site (site/dist) uses exactly one icon source: apps/kira-space/build/appicon.png. */
import { readFileSync } from 'node:fs';
import { basename, join, relative } from 'node:path';
import { Glob } from 'bun';
import sharp from 'sharp';

const SITE = join(import.meta.dir, '..');
const ROOT = join(SITE, '..');
const DIST = join(SITE, 'dist');
const SOURCE = join(ROOT, 'apps', 'kira-space', 'build', 'appicon.png');
const FAVICON = '/kira-space/favicon.png';

const print = (line: string) => process.stderr.write(`${line}\n`);
let violations = 0;
const fail = (file: string, reason: string) => {
  print(`check-icons: ${file}: ${reason}`);
  violations += 1;
};

const allPages = [...new Glob('**/*.html').scanSync({ cwd: DIST })].sort();
// Redirect stubs (meta refresh) render nothing, so they carry no icon.
const pages = allPages.filter(
  (p) => !/http-equiv="refresh"/i.test(readFileSync(join(DIST, p), 'utf8')),
);
const iconHrefs = new Set<string>();
for (const page of pages) {
  const html = readFileSync(join(DIST, page), 'utf8');
  const links = [...html.matchAll(/<link\b[^>]*>/g)].map((m) => m[0]);
  const attr = (tag: string, name: string) =>
    new RegExp(`\\s${name}=(?:"([^"]*)"|'([^']*)'|([^\\s>]+))`).exec(tag)?.slice(1).find(Boolean);
  const icons = links.filter((tag) => /icon/i.test(attr(tag, 'rel') ?? ''));
  if (icons.length === 0) fail(page, 'no <link rel> containing icon');
  for (const tag of icons) {
    const rel = attr(tag, 'rel') ?? '';
    const href = attr(tag, 'href') ?? '';
    if (rel.split(/\s+/).includes('apple-touch-icon')) {
      if (!/^\/kira-space\/_astro\/appicon\.[^/]+\.png$/.test(href)) {
        fail(page, `apple-touch-icon href ${href} is not the appicon asset`);
      }
    } else {
      iconHrefs.add(href);
    }
  }
  for (const m of html.matchAll(/\/kira-space\/_astro\/([^"')\s?#]+)/g)) {
    const name = m[1] ?? '';
    const stem = name.split('.')[0] ?? '';
    if (/icon|logo/i.test(stem) && stem !== 'appicon') {
      fail(page, `${name} is an icon or logo asset other than appicon`);
    }
  }
}
if (iconHrefs.size !== 1 || !iconHrefs.has(FAVICON)) {
  fail('site/dist', `icon hrefs are {${[...iconHrefs].join(', ')}}, expected {${FAVICON}}`);
}

for (const file of new Glob('**/*').scanSync({ cwd: DIST })) {
  const name = basename(file);
  if (name === 'favicon.svg' || name === 'favicon.ico' || name.includes('kira_icon_vector')) {
    fail(relative(ROOT, join(DIST, file)), 'second icon source');
  }
}

try {
  const built = sharp(join(DIST, 'favicon.png'));
  const meta = await built.metadata();
  if (meta.width !== 64 || meta.height !== 64) {
    fail('site/dist/favicon.png', `size ${meta.width}x${meta.height}, expected 64x64`);
  }
  const want = await sharp(SOURCE).resize(64, 64).ensureAlpha().raw().toBuffer();
  const got = await built.ensureAlpha().raw().toBuffer();
  if (!want.equals(got))
    fail('site/dist/favicon.png', 'pixels differ from the resized appicon.png');
} catch (err) {
  fail('site/dist/favicon.png', `unreadable: ${err instanceof Error ? err.message : String(err)}`);
}

print(`check-icons: ${pages.length} pages, ${violations} violations`);
process.exit(violations > 0 ? 1 : 0);
