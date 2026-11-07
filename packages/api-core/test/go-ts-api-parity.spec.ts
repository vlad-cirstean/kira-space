// These three checks (body modes, content types, Postman code languages) are Api-only, so they
// move beside the package whose vocabulary they check. Same technique as
// tests/unit/go-ts-vocabulary-parity.spec.ts: reading the Go source as plain text rather than
// generating one side from the other.

import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import {
  CODE_LANGUAGES,
  CONTENT_TYPE_BY_CODE_LANGUAGE,
  HTTP_BODY_MODES,
} from '@kira/shared/domain/http';
import { ALIAS_TO_FAKE } from '../src/http/dynamic/catalog';
import { TRANSFORM_NAMES } from '../src/http/transforms';

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

/** P3 D12: the map[string]string sibling of extractGoStringSet — pulls every `"key": "value"`
 *  entry out of a Go `var <name> = map[string]string{ ... }` literal. */
function extractGoStringMap(source: string, varName: string): Record<string, string> {
  const stripped = stripGoLineComments(source);
  const header = new RegExp(`var\\s+${varName}\\s*=\\s*map\\[string\\]string\\{`);
  const headerMatch = header.exec(stripped);
  if (!headerMatch) {
    throw new Error(`could not find "var ${varName} = map[string]string{" in the Go source`);
  }
  const bodyStart = headerMatch.index + headerMatch[0].length;
  const bodyEnd = findMapBodyEnd(stripped, bodyStart);
  if (bodyEnd < 0) throw new Error(`unterminated "${varName}" map literal`);
  const body = stripped.slice(bodyStart, bodyEnd);
  const out: Record<string, string> = {};
  for (const m of body.matchAll(/"([^"]+)":\s*"([^"]*)"/g)) out[m[1]] = m[2];
  if (Object.keys(out).length === 0)
    throw new Error(`"${varName}" map literal parsed to zero entries`);
  return out;
}

// The same silent-drift shape go-ts-vocabulary-parity.spec.ts's own tab/op-kind checks
// guard, for the request body's own two small vocabularies — the mode strings are duplicated in
// two languages, and so are the Content-Type-by-code-language values (Go decides what to send;
// body.ts decides what the caption claims will be sent).
describe('Go/TS HTTP body vocabulary parity (P3 D12)', () => {
  test('httpclient.validBodyModes (Go) matches HTTP_BODY_MODES minus the legacy json alias (TS)', () => {
    const source = readFileSync(
      resolve(import.meta.dir, '../../../apps/kira-space/internal/httpclient/body.go'),
      'utf8',
    );
    const goModes = extractGoStringSet(source, 'validBodyModes');
    // 'json' is a presentation-only segment (api-core body.ts) and
    // deliberately has no Go counterpart — HTTP_BODY_MODES itself never contains it.
    expect(goModes).toEqual(new Set(HTTP_BODY_MODES));
  });

  test('httpclient.contentTypeByCodeLanguage (Go) matches CONTENT_TYPE_BY_CODE_LANGUAGE (TS)', () => {
    const source = readFileSync(
      resolve(import.meta.dir, '../../../apps/kira-space/internal/httpclient/body.go'),
      'utf8',
    );
    const goTable = extractGoStringMap(source, 'contentTypeByCodeLanguage');
    expect(goTable).toEqual(CONTENT_TYPE_BY_CODE_LANGUAGE);
  });
});

// The Postman translation table lives only in Go, so most of it needs no parity guard.
// One list does. If a fifth code language were ever added on the TypeScript side,
// internal/postman/body.go's importer would silently stop recognising it — importing that
// language's bodies as plain `raw` — and its exporter would silently stop emitting it, with
// nothing failing anywhere.
describe('Go/TS Postman code-language parity (P4 D17)', () => {
  test('postman.postmanCodeLanguages (Go) matches CODE_LANGUAGES (TS)', () => {
    const source = readFileSync(
      resolve(import.meta.dir, '../../../apps/kira-space/internal/postman/body.go'),
      'utf8',
    );
    const goLanguages = extractGoStringSet(source, 'postmanCodeLanguages');
    expect(goLanguages).toEqual(new Set(CODE_LANGUAGES));
  });
});

// The same silent-drift shape as the two describes above, for the six-transform closed
// vocabulary a `{{name | transform}}` pipe may apply — apivars/transforms.go's own comment
// names this exact test as the reason its `transformNames` map literal must keep this shape. A
// transform added on one side and not the other is a failing test here, rather than a pipeline
// that resolves in the renderer's live preview and silently falls back to `unknown` (or vice
// versa) only once a request reaches Go's stage 2.
describe('Go/TS transform vocabulary parity (P17 D7)', () => {
  test('apivars.transformNames (Go) matches TRANSFORM_NAMES (TS)', () => {
    const source = readFileSync(
      resolve(import.meta.dir, '../../../apps/kira-space/internal/apivars/transforms.go'),
      'utf8',
    );
    const goNames = extractGoStringSet(source, 'transformNames');
    expect(goNames).toEqual(new Set(TRANSFORM_NAMES));
  });
});

// The Postman \`{{$name}}\` -> \`fake.\` mapping now exists twice — as ALIAS_TO_FAKE here
// (what the app resolves and what the catalogue teaches) and as postman.aliasToFake in Go (what an
// import rewrites with). Drift is silent in the worst direction: an alias missing from the Go table
// imports verbatim and then, because this app no longer offers or documents the \`$\` spelling,
// looks to the user like a reference nothing in the app knows about.
describe('Go/TS Postman dynamic-alias parity (P28 D15c)', () => {
  test('postman.aliasToFake (Go) matches ALIAS_TO_FAKE (TS)', () => {
    const source = readFileSync(
      resolve(import.meta.dir, '../../../apps/kira-space/internal/postman/aliases.go'),
      'utf8',
    );
    const goTable = extractGoStringMap(source, 'aliasToFake');
    expect(goTable).toEqual(ALIAS_TO_FAKE);
  });
});
