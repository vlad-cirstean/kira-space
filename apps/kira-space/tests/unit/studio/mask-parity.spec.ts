// The cross-language parity pin — tests/fixtures/studio/mask/*.{input,expected}.json, generated
// from internal/mask (the queryplan/parse_test.go + explain-plan.spec.ts precedent). Two
// independent ports of one algorithm drifting silently is the failure this guards, and it is the
// only thing that can catch it.

import { describe, expect, test } from 'bun:test';
import { readdirSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import type { MaskingRule } from '@shared/domain/mask';
import { maskNullable } from '@shared/domain/mask';

const DIR = join(dirname(fileURLToPath(import.meta.url)), '../../fixtures/studio/mask');

interface FixtureRule {
  kind: string;
  keepHint: boolean;
  correlate: boolean;
}

interface FixtureInput {
  rule: FixtureRule;
  key: string | null; // hex-encoded
  value: string | null;
}

interface FixtureExpected {
  output: string | null;
}

function fixtureCases(): string[] {
  return readdirSync(DIR)
    .filter((f) => f.endsWith('.input.json'))
    .map((f) => f.replace(/\.input\.json$/, ''))
    .sort();
}

function loadInput(caseName: string): FixtureInput {
  return JSON.parse(readFileSync(join(DIR, `${caseName}.input.json`), 'utf8'));
}

function loadExpected(caseName: string): FixtureExpected {
  return JSON.parse(readFileSync(join(DIR, `${caseName}.expected.json`), 'utf8'));
}

// hexToBytes is a browser-safe decode (no node:buffer) — the same constraint the real renderer
// runtime has, since Uint8Array is what crypto.subtle.importKey needs everywhere.
function hexToBytes(hex: string): Uint8Array {
  const bytes = new Uint8Array(hex.length / 2);
  for (let i = 0; i < bytes.length; i++) {
    bytes[i] = Number.parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  }
  return bytes;
}

const cases = fixtureCases();

describe('mask parity fixtures', () => {
  test('at least one fixture case was found', () => {
    expect(cases.length).toBeGreaterThan(0);
  });

  for (const name of cases) {
    test(name, async () => {
      const input = loadInput(name);
      const expected = loadExpected(name);
      const rule: MaskingRule = {
        kind: input.rule.kind as MaskingRule['kind'],
        keepHint: input.rule.keepHint,
        correlate: input.rule.correlate,
      };
      const key = input.key === null ? null : hexToBytes(input.key);
      const got = await maskNullable(rule, key, input.value);
      expect(got).toBe(expected.output);
    });
  }
});
