#!/usr/bin/env bun
/**
 * P550: checks that every shot in shotCapture.ts has its image under screenshots/.
 * Strict (default): a missing file prints the list and exits 1.
 * `--fallback <dir>` (CI): copies a missing file from the last-good cache, else switches the site
 * build to placeholders; writes `complete` to $GITHUB_OUTPUT and always exits 0.
 */
import { appendFileSync, copyFileSync, existsSync, mkdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { SHOT_CAPTURE, type ShotId, shotFiles } from '../src/data/shotCapture';

const OUT = join(import.meta.dir, '..', '..', 'screenshots');
const KINDS = new Set(['window', 'floating', 'region']);

const print = (line: string) => process.stderr.write(`${line}\n`);
const arg = process.argv.indexOf('--fallback');
const fallback = arg === -1 ? undefined : process.argv[arg + 1];
if (arg !== -1 && !fallback) {
  print('shots-fill: --fallback needs a directory');
  process.exit(1);
}

const ids = Object.keys(SHOT_CAPTURE) as ShotId[];
const owner = new Map<string, ShotId>();
for (const id of ids) {
  const kind: string = SHOT_CAPTURE[id].kind;
  if (!KINDS.has(kind)) {
    print(`shots-fill: ${id} has unknown kind ${kind}`);
    process.exit(1);
  }
  const { main, phone } = shotFiles(id);
  for (const file of phone ? [main, phone] : [main]) {
    const other = owner.get(file);
    if (other) {
      print(`shots-fill: ${id} and ${other} both write ${file}`);
      process.exit(1);
    }
    owner.set(file, id);
  }
}

const filled: string[] = [];
const missing: string[] = [];
for (const [file, id] of owner) {
  const target = join(OUT, file);
  if (existsSync(target)) continue;
  const cached = fallback ? join(fallback, file) : undefined;
  if (cached && existsSync(cached)) {
    mkdirSync(dirname(target), { recursive: true });
    copyFileSync(cached, target);
    filled.push(`${id}: ${file}`);
    print(`::warning::Shot ${id} not captured, using the last-good copy of ${file}`);
  } else {
    missing.push(`${id}: ${file}`);
    print(
      fallback
        ? `::warning::Shot ${id} not captured, no copy of ${file}`
        : `Missing ${file} (${id})`,
    );
  }
}

if (!fallback) {
  if (missing.length > 0) {
    print(`shots-fill: ${missing.length} missing. Run: bun run screenshots`);
    process.exit(1);
  }
  print(`shots-fill: ${owner.size} files for ${ids.length} shots`);
  process.exit(0);
}

const append = (name: string, line: string) => {
  const path = process.env[name];
  if (path) appendFileSync(path, `${line}\n`);
};
if (missing.length > 0) append('GITHUB_ENV', 'KIRA_SITE_SHOTS=placeholder');
append('GITHUB_OUTPUT', `complete=${filled.length === 0 && missing.length === 0}`);
if (filled.length > 0 || missing.length > 0) {
  append('GITHUB_STEP_SUMMARY', '### Site shots');
  for (const line of filled) append('GITHUB_STEP_SUMMARY', `- last-good copy: ${line}`);
  for (const line of missing) append('GITHUB_STEP_SUMMARY', `- placeholder: ${line}`);
}
