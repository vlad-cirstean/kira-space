#!/usr/bin/env bun
/**
 * P674 UI audit: finds reimplementation and display drift across the whole frontend.
 * Discovery only: heuristic and noisy by design, never wired into `lint` or hooks. Enforcement
 * lives in the guard scripts. Stateless: reads no ledger, allowlist or baseline.
 *
 *   bun run audit:ui [--channel c1|...|c7] [--out <file>]
 */
import { writeFileSync } from 'node:fs';
import { runClasses } from './classes';
import { runDisplay } from './display';
import { loadFiles } from './files';
import { runLibraries } from './libraries';
import { runNearMiss } from './nearmiss';
import { type ChannelResult, renderReport } from './report';
import { runScripts } from './scripts';
import { runTemplates } from './templates';
import { runVocabulary } from './vocabulary';

const CHANNELS: Record<string, () => ChannelResult> = {
  c1: runTemplates,
  c2: runClasses,
  c3: runScripts,
  c4: runNearMiss,
  c5: runDisplay,
  c6: runVocabulary,
  c7: runLibraries,
};

function arg(name: string): string | undefined {
  const i = process.argv.indexOf(`--${name}`);
  return i >= 0 ? process.argv[i + 1] : undefined;
}

const only = arg('channel')?.toLowerCase();
if (only && !CHANNELS[only]) {
  console.error(`uiaudit: unknown channel "${only}"; use ${Object.keys(CHANNELS).join(', ')}`);
  process.exit(2);
}

const started = performance.now();
const files = loadFiles();
const results: ChannelResult[] = [];
for (const [id, run] of Object.entries(CHANNELS)) {
  if (only && id !== only) continue;
  const t0 = performance.now();
  const r = run();
  results.push(r);
  console.error(
    `uiaudit: ${id} ${r.clusters.length} clusters in ${Math.round(performance.now() - t0)} ms`,
  );
}
const report = renderReport(results, {
  files: files.length,
  vue: files.filter((f) => f.ext === 'vue').length,
  ts: files.filter((f) => f.ext === 'ts').length,
});
const out = arg('out');
if (out) writeFileSync(out, report);
else process.stdout.write(report);
console.error(`uiaudit: done in ${((performance.now() - started) / 1000).toFixed(1)} s`);
