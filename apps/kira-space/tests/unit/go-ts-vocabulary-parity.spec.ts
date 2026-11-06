// Kira Space's own half of the Go/TS tab-kind vocabulary parity check —
// Kira Studio's own copy (apps/kira-space/tests/unit/go-ts-vocabulary-parity.spec.ts) used to
// check the one shared 16-member RENDERABLE_TAB_KINDS against Kira Studio's Go list alone,
// leaving this app's own five-kind vocabulary unchecked against anything. This is the one
// vocabulary TypeScript's own exhaustiveness checks cannot catch a miss on — a kind missing from
// either side is dropped on restore with a `warn` nobody reads (repos/tabs.go's own comment).

import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { SPACE_RENDERABLE_TAB_KINDS } from '../../frontend/src/state/tabDomain';

/** Pulls every `"key": true` entry out of a Go `var <name> = map[string]bool{ ... }` literal —
 *  tolerant of comments and multi-entries-per-line, not a full Go parser. Kira Studio's own copy
 *  of this helper, verbatim. */
function extractGoStringSet(source: string, varName: string): Set<string> {
  const header = new RegExp(`var\\s+${varName}\\s*=\\s*map\\[string\\]bool\\{`);
  const headerMatch = header.exec(source);
  if (!headerMatch) {
    throw new Error(`could not find "var ${varName} = map[string]bool{" in the Go source`);
  }
  const bodyStart = headerMatch.index + headerMatch[0].length;
  const bodyEnd = source.indexOf('}', bodyStart);
  if (bodyEnd < 0) throw new Error(`unterminated "${varName}" map literal`);
  const body = source.slice(bodyStart, bodyEnd);
  return new Set([...body.matchAll(/"([^"]+)":\s*true/g)].map((m) => m[1]));
}

describe('Go/TS tab-kind vocabulary parity (P103 Part 2 §5.1)', () => {
  test('model.RenderableTabKinds (Go) matches SPACE_RENDERABLE_TAB_KINDS (TS)', () => {
    const source = readFileSync(
      resolve(import.meta.dir, '../../internal/storage/model/tabs.go'),
      'utf8',
    );
    const goKinds = extractGoStringSet(source, 'RenderableTabKinds');
    expect(goKinds).toEqual(new Set(SPACE_RENDERABLE_TAB_KINDS));
  });
});
