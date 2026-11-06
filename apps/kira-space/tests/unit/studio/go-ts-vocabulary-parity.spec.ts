// Adding a tab kind or an op kind touches four vocabularies:
// tabKindSchema, RENDERABLE_TAB_KINDS and tabRecordSchema's discriminated union on the TS side
// (all three exhaustiveness-checked by the compiler), plus Go's own model.RenderableTabKinds/
// opKinds — the one of the four with no compiler behind it. A kind missing there is dropped on
// restore with a `warn` nobody reads (repos/tabs.go's own comment). Generating one side from the
// other was declined ("too much machinery for two lists of eight strings") in favour of
// reading the Go source as plain text, the same technique this repo already leans on elsewhere to
// cross-check a generated/hand-written pair without a build step.
//
// This file's own vocabulary — tab kinds and op kinds — spans both Studio and Api by
// nature, so it stays a host-level spec. The Api-only body/content-type/Postman-language parity
// checks that used to live below move to packages/api-core/test/go-ts-api-parity.spec.ts, next to
// the package whose vocabulary they check.

import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { GRPC_HISTORY_PER_SCOPE_LIMIT } from '../../../../../packages/shared/domain/grpcHistory';
import { opKindSchema } from '../../../../../packages/shared/domain/ops';
import { HISTORY_PER_SCOPE_LIMIT } from '../../../../../packages/shared/domain/responseHistory';
import { STUDIO_RENDERABLE_TAB_KINDS } from '../../../frontend/src/studio/state/tabDomain';

/** Strips a Go `//` line comment from each line before any extractor scans the source —
 *  without this, a `}` or a `name = N` pattern inside a comment can end a map body early or satisfy
 *  extractGoIntConst's match instead of the real declaration. None of this file's map/const
 *  declarations put `//` inside a string literal, so a per-line strip is sufficient. */
function stripGoLineComments(source: string): string {
  return source
    .split('\n')
    .map((line) => line.replace(/\/\/.*$/, ''))
    .join('\n');
}

/** Finds the `}` that closes the map literal opened at `bodyStart` (already past
 *  its `{`), depth-counting braces and skipping over string-literal contents — `source.indexOf('}',
 *  bodyStart)` stopped at the first `}` at any depth, which a nested struct value or a `}` inside a
 *  quoted string would close early. */
function findMapBodyEnd(source: string, bodyStart: number): number {
  let depth = 1;
  let inString = false;
  for (let i = bodyStart; i < source.length; i++) {
    const c = source[i];
    if (inString) {
      if (c === '\\') {
        i++;
        continue;
      }
      if (c === '"') inString = false;
      continue;
    }
    if (c === '"') {
      inString = true;
      continue;
    }
    if (c === '{') depth++;
    else if (c === '}') {
      depth--;
      if (depth === 0) return i;
    }
  }
  return -1;
}

/** Pulls every `"key": true` entry out of a Go `var <name> = map[string]bool{ ... }` literal —
 *  tolerant of comments and multi-entries-per-line (both present in the real source), not a full
 *  Go parser. */
function extractGoStringSet(source: string, varName: string): Set<string> {
  const stripped = stripGoLineComments(source);
  const header = new RegExp(`var\\s+${varName}\\s*=\\s*map\\[string\\]bool\\{`);
  const headerMatch = header.exec(stripped);
  if (!headerMatch) {
    throw new Error(`could not find "var ${varName} = map[string]bool{" in the Go source`);
  }
  const bodyStart = headerMatch.index + headerMatch[0].length;
  const bodyEnd = findMapBodyEnd(stripped, bodyStart);
  if (bodyEnd < 0) throw new Error(`unterminated "${varName}" map literal`);
  const body = stripped.slice(bodyStart, bodyEnd);
  const result = new Set([...body.matchAll(/"([^"]+)":\s*true/g)].map((m) => m[1]));
  if (result.size === 0) throw new Error(`"${varName}" map literal parsed to zero entries`);
  return result;
}

/** Pulls a `<name> = <integer>` constant literal out of a Go source file —
 *  tolerant of it living inside a `const ( ... )` block with a comment above it. Matched
 *  against comment-stripped source, so a comment mentioning "name = N" in prose can't satisfy this
 *  before the real declaration does. */
function extractGoIntConst(source: string, constName: string): number {
  const stripped = stripGoLineComments(source);
  const m = new RegExp(`\\b${constName}\\s*=\\s*(\\d+)\\b`).exec(stripped);
  if (!m) throw new Error(`could not find "${constName} = <n>" in the Go source`);
  return Number(m[1]);
}

describe('Go/TS tab- and op-kind vocabulary parity (P2 D10)', () => {
  // This used to check the one shared 16-member RENDERABLE_TAB_KINDS against Kira Studio's own Go
  // list — which passed only because both sides were equally wrong (Kira Studio's Go list still
  // carried four kinds it can no longer produce, and nothing checked Kira Space at all). Now checks
  // this app's own 12-member STUDIO_RENDERABLE_TAB_KINDS; Kira Space's own 5-member half lives at
  // apps/kira-space/tests/unit/go-ts-vocabulary-parity.spec.ts.
  test('model.RenderableTabKinds (Go) matches STUDIO_RENDERABLE_TAB_KINDS (TS)', () => {
    const source = readFileSync(
      resolve(import.meta.dir, '../../../internal/studiostorage/model/tabs.go'),
      'utf8',
    );
    const goKinds = extractGoStringSet(source, 'RenderableTabKinds');
    expect(goKinds).toEqual(new Set(STUDIO_RENDERABLE_TAB_KINDS));
  });

  test('model.opKinds (Go) matches opKindSchema (TS)', () => {
    const source = readFileSync(
      resolve(import.meta.dir, '../../../internal/studiostorage/model/ops.go'),
      'utf8',
    );
    const goKinds = extractGoStringSet(source, 'opKinds');
    expect(goKinds).toEqual(new Set(opKindSchema.options));
  });
});

describe('Go/TS history retention cap parity (P18 D4)', () => {
  test('response_history.go historyPerScopeLimit matches HISTORY_PER_SCOPE_LIMIT (TS)', () => {
    const source = readFileSync(
      resolve(import.meta.dir, '../../../internal/studiostorage/repos/response_history.go'),
      'utf8',
    );
    expect(extractGoIntConst(source, 'historyPerScopeLimit')).toBe(HISTORY_PER_SCOPE_LIMIT);
  });

  test('grpc_history.go grpcHistoryPerScopeCap matches GRPC_HISTORY_PER_SCOPE_LIMIT (TS)', () => {
    const source = readFileSync(
      resolve(import.meta.dir, '../../../internal/studiostorage/repos/grpc_history.go'),
      'utf8',
    );
    expect(extractGoIntConst(source, 'grpcHistoryPerScopeCap')).toBe(GRPC_HISTORY_PER_SCOPE_LIMIT);
  });
});
