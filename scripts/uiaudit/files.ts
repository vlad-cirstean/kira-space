import { readdirSync, readFileSync } from 'node:fs';
import { join, relative } from 'node:path';
import { parse as parseSFC, type SFCDescriptor } from '@vue/compiler-sfc';
import { moduleOf } from './modules';

export const REPO_ROOT = join(import.meta.dir, '..', '..');

export interface SourceFile {
  path: string;
  module: string;
  ext: 'vue' | 'ts';
  source: string;
  /** Vue SFC descriptor, absent for `.ts` and for unparsable files. */
  descriptor: SFCDescriptor | null;
}

/** `DIRS` and `CORE_DIRS` of check-ui-primitives.sh: the guard script stays the one list. */
function scanDirs(): string[] {
  const script = readFileSync(join(REPO_ROOT, 'scripts/check-ui-primitives.sh'), 'utf8');
  const dirs: string[] = [];
  for (const name of ['DIRS', 'CORE_DIRS']) {
    const m = new RegExp(`^${name}="([^"]+)"`, 'm').exec(script);
    if (!m) throw new Error(`uiaudit: ${name} not found in check-ui-primitives.sh`);
    dirs.push(...m[1].split(/\s+/));
  }
  return dirs;
}

const EXCLUDED = [
  /^packages\/theme\/src\/components\/ui\//,
  /\/testing\//,
  /\.spec\.ts$/,
  /\.d\.ts$/,
  /\/(wire|bindings|generated)\//,
];

export function isScanned(rel: string): boolean {
  return (rel.endsWith('.vue') || rel.endsWith('.ts')) && !EXCLUDED.some((r) => r.test(rel));
}

function walk(dir: string, out: string[]): void {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (entry.name === 'node_modules' || entry.name === 'dist' || entry.name.startsWith('.')) {
      continue;
    }
    const full = join(dir, entry.name);
    if (entry.isDirectory()) walk(full, out);
    else out.push(full);
  }
}

let cache: SourceFile[] | undefined;

export function loadFiles(): SourceFile[] {
  if (cache) return cache;
  const abs: string[] = [];
  for (const d of scanDirs()) walk(join(REPO_ROOT, d), abs);
  const files: SourceFile[] = [];
  for (const a of abs.sort()) {
    const rel = relative(REPO_ROOT, a);
    if (!isScanned(rel)) continue;
    const source = readFileSync(a, 'utf8');
    const ext = rel.endsWith('.vue') ? 'vue' : 'ts';
    let descriptor: SFCDescriptor | null = null;
    if (ext === 'vue') {
      try {
        descriptor = parseSFC(source, { filename: rel }).descriptor;
      } catch {
        descriptor = null;
      }
    }
    files.push({ path: rel, module: moduleOf(rel), ext, source, descriptor });
  }
  cache = files;
  return files;
}
