#!/usr/bin/env bun
/**
 * P265 F3: catalog checker. Loads packages/theme/src/i18n/en.json and runs rules C1-C11
 * (scripts/copy/catalogRules.ts). `--unused` adds C10 (key never referenced from a source file).
 */
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { checkCatalog, type UsedKeys } from './catalogRules';

const REPO_ROOT = join(import.meta.dir, '..', '..');
const CATALOG = join(REPO_ROOT, 'packages/theme/src/i18n/en.json');
const UNUSED_SCAN = ['apps', 'packages', 'internal'];
const SKIP_DIR = /(?:^|\/)(?:node_modules|dist|dist-test|\.git|tests?|testing)(?:\/|$)/;
const SKIP_FILE = /(?:\.spec\.ts|\.test\.ts|_test\.go|\.d\.ts|\.check\.ts)$/;
const KEY_LITERAL = /["'`]([a-z][a-zA-Z0-9]*(?:\.[a-zA-Z0-9]+){2,5})["'`]/g;
const KEY_PREFIX = /`([a-z][a-zA-Z0-9]*(?:\.[a-zA-Z0-9]+)*\.)\$\{/g;

function collectUsed(): UsedKeys {
  const literals = new Set<string>();
  const prefixes = new Set<string>();
  for (const root of UNUSED_SCAN) {
    const base = join(REPO_ROOT, root);
    for (const rel of readdirSync(base, { recursive: true, encoding: 'utf8' })) {
      if (SKIP_DIR.test(rel) || SKIP_FILE.test(rel) || !/\.(?:vue|ts|go)$/.test(rel)) continue;
      const text = readFileSync(join(base, rel), 'utf8');
      for (const m of text.matchAll(KEY_LITERAL)) literals.add(m[1] as string);
      for (const m of text.matchAll(KEY_PREFIX)) prefixes.add(m[1] as string);
    }
  }
  return { literals, prefixes: [...prefixes] };
}

function main(): number {
  const unused = process.argv.includes('--unused');
  const root: unknown = JSON.parse(readFileSync(CATALOG, 'utf8'));
  const violations = checkCatalog(root, unused ? { used: collectUsed() } : {});
  violations.sort((a, b) => a.key.localeCompare(b.key) || a.rule.localeCompare(b.rule));
  for (const v of violations) console.error(`check-catalog: ${v.key}: ${v.rule} ${v.message}`);
  if (violations.length > 0) {
    console.error(`check-catalog: ${violations.length} violation(s)`);
    return 1;
  }
  return 0;
}

process.exit(main());
