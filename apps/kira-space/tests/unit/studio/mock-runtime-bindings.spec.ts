import { describe, expect, test } from 'bun:test';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

// Every bound method of a service only Studio calls needs a mock entry (an unmapped one answers
// E_FIXTURE_MISS) and every Studio entry must name a real method (a renamed binding silently strands
// its fixtures). Services Space also maps are merged: each app's table covers just its own methods.
const BINDINGS_DIR = join(
  import.meta.dir,
  '../../../frontend/bindings/github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge',
);

// Read as text: the module itself pulls in the Playwright-only `@workbench/testing` alias, which
// bun's test resolver does not carry.
function mappedMethods(file: string): Set<string> {
  const source = readFileSync(join(import.meta.dir, file), 'utf8');
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
  const mapped = mappedMethods('../../ui/studio/support/mockRuntime.ts');
  const spaceServices = new Set(
    [...mappedMethods('../../ui/support/mockRuntime.ts')].map((fqn) => fqn.split('.')[0]),
  );

  const services = new Set(
    [...mapped].map((fqn) => fqn.split('.')[0]).filter((svc) => !spaceServices.has(svc)),
  );

  test('every bound method is mapped', () => {
    expect(
      [...bound].filter((fqn) => services.has(fqn.split('.')[0]) && !mapped.has(fqn)).sort(),
    ).toEqual([]);
  });

  test('every mapped method is bound', () => {
    expect([...mapped].filter((fqn) => !bound.has(fqn)).sort()).toEqual([]);
  });
});
