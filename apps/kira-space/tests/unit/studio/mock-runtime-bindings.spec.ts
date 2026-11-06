import { describe, expect, test } from 'bun:test';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

// Every bound Go method needs a mock entry (an unmapped one answers E_FIXTURE_MISS) and every entry
// must name a real method (a renamed binding silently strands its fixtures).
const BINDINGS_DIR = join(
  import.meta.dir,
  '../../../frontend/bindings/github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge',
);

// Read as text: the module itself pulls in the Playwright-only `@workbench/testing` alias, which
// bun's test resolver does not carry.
function mappedMethods(): Set<string> {
  const source = readFileSync(join(import.meta.dir, '../../ui/studio/support/mockRuntime.ts'), 'utf8');
  const start = source.indexOf('const FQN_SUFFIX_BY_IPC_KEY');
  const end = source.indexOf('\n};', start);
  return new Set(
    [...source.slice(start, end).matchAll(/: '([A-Za-z]+Service\.[A-Za-z]+)'/g)].map((m) => m[1]),
  );
}

function boundMethods(): Set<string> {
  const out = new Set<string>();
  for (const file of readdirSync(BINDINGS_DIR).filter((f) => f.endsWith('.ts'))) {
    const text = readFileSync(join(BINDINGS_DIR, file), 'utf8');
    for (const m of text.matchAll(/\$Call\.ByName\("([^"]+)"/g)) out.add(m[1]);
  }
  return out;
}

describe('mockRuntime FQN table matches the generated bindings', () => {
  const bound = new Set([...boundMethods()].map((fqn) => fqn.split('/bridge.')[1]));
  const mapped = mappedMethods();

  test('every bound method is mapped', () => {
    expect([...bound].filter((fqn) => !mapped.has(fqn)).sort()).toEqual([]);
  });

  test('every mapped method is bound', () => {
    expect([...mapped].filter((fqn) => !bound.has(fqn)).sort()).toEqual([]);
  });
});
