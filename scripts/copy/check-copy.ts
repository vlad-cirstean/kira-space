#!/usr/bin/env bun
/**
 * P265 F3: source text guard U64-U71 and U182 (catalog key ownership, ownership.ts). Flags literal user-visible text and misuse of the catalog
 * runtime in `.vue` and `.ts` sources (rules in sourceRules.ts). Exemptions are named, with a
 * reason, in exemptions.ts.
 */
import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { applyExemptions } from './allowlist';
import { flattenCatalog, isPlural, placeholdersOf } from './catalogRules';
import { EXEMPTIONS, OWNERSHIP_PAIRS } from './exemptions';
import { applyPairs, type OwnershipHit, scanOwnership } from './ownership';
import { type Hit, type MessageShape, scanSource } from './sourceRules';

const REPO_ROOT = join(import.meta.dir, '..', '..');
const SCAN_DIRS = [
  'apps/kira-space/frontend/src',
  'apps/kira-space/frontend/mobile',
  'packages/workbench/src',
  'packages/git-ui/src',
  'packages/docker-ui/src',
  'packages/theme/src',
  'packages/shared',
  'packages/git-core/src',
  'packages/git-ipc/src',
  'packages/api-core/src',
];
const SKIP_PATH =
  /(?:^|\/)(?:node_modules|dist|testing|tests?|__tests__)\/|^packages\/theme\/src\/i18n\/|^packages\/shared\/protocol\/wire(?:\/|\.ts$)|\.(?:spec|test)\.ts$|\.d\.ts$/;
const CATALOG = join(REPO_ROOT, 'packages/theme/src/i18n/en.json');
const MAX_PRINTED = 300;

function listSources(): string[] {
  const out: string[] = [];
  for (const dir of SCAN_DIRS) {
    const base = join(REPO_ROOT, dir);
    if (!existsSync(base)) continue;
    for (const rel of readdirSync(base, { recursive: true, encoding: 'utf8' })) {
      const path = `${dir}/${rel}`;
      if (/\.(?:vue|ts)$/.test(rel) && !SKIP_PATH.test(path)) out.push(path);
    }
  }
  return out.sort();
}

function loadMessages(): Map<string, MessageShape> {
  const shapes = new Map<string, MessageShape>();
  if (!existsSync(CATALOG)) return shapes;
  const { catalog } = flattenCatalog(JSON.parse(readFileSync(CATALOG, 'utf8')));
  for (const [key, value] of catalog) {
    shapes.set(key, { placeholders: placeholdersOf(value), plural: isPlural(value) });
  }
  return shapes;
}

function main(): number {
  const messages = loadMessages();
  const keys = new Set(messages.keys());
  const hits: Hit[] = [];
  const owned: OwnershipHit[] = [];
  for (const file of listSources()) {
    const source = readFileSync(join(REPO_ROOT, file), 'utf8');
    hits.push(...scanSource(file, source, { messages }));
    owned.push(...scanOwnership(file, source, keys));
  }
  const { remaining, errors } = applyExemptions(hits, EXEMPTIONS);
  const pairs = applyPairs(owned, OWNERSHIP_PAIRS);
  errors.push(...pairs.errors);
  const limit = process.argv.includes('--all') ? remaining.length : MAX_PRINTED;
  for (const h of remaining.slice(0, limit)) {
    console.error(`check-copy: ${h.file}:${h.line}: ${h.rule} ${h.message}`);
  }
  if (remaining.length > MAX_PRINTED)
    console.error(`check-copy: ...and ${remaining.length - MAX_PRINTED} more`);
  for (const h of pairs.remaining) {
    console.error(
      `check-copy: ${h.file}:${h.line}: U182 '${h.key}' belongs to another module; move it to common.* or name a pair`,
    );
  }
  for (const e of errors) console.error(`check-copy: ${e}`);
  const total = remaining.length + pairs.remaining.length;
  if (total > 0 || errors.length > 0) {
    const files = new Set([...remaining, ...pairs.remaining].map((h) => h.file)).size;
    console.error(
      `check-copy: ${total} hit(s) in ${files} file(s), ${errors.length} exemption error(s)`,
    );
    return 1;
  }
  return 0;
}

process.exit(main());
