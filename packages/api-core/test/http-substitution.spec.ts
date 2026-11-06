// The {{name}} substitution engine is written twice — once here (src/http/substitute.ts)
// and once in Go (internal/apivars/resolve.go) — and pinned to the same behaviour by one shared
// corpus, read by both a Go test and this one, rather than by two independently-imagined test
// suites. A case added to the corpus must pass on both sides or one of them fails.

import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { resolve as resolvePath } from 'node:path';
import {
  classifyReference,
  parseReference,
  type Reference,
  resolve,
  sanitizeUrlSpan,
  splitTemplateSpans,
} from '../src/http/substitute';
import { applyPipeline } from '../src/http/transforms';

interface Case {
  name: string;
  template: string;
  values: Record<string, string>;
  secrets: string[];
  want: string;
  refs: Reference[];
}

const cases: Case[] = JSON.parse(
  readFileSync(
    resolvePath(
      import.meta.dir,
      '../../../apps/kira-space/internal/apivars/testdata/substitution.json',
    ),
    'utf8',
  ),
);

describe('http/substitute.ts (P5 D17/D18)', () => {
  test('the corpus is non-empty', () => {
    expect(cases.length).toBeGreaterThan(0);
  });

  for (const c of cases) {
    test(c.name, () => {
      const result = resolve(c.template, c.values, c.secrets);
      expect(result.text).toBe(c.want);
      expect(result.refs).toEqual(c.refs);
    });
  }
});

// P6 D2/D3: three TS-only cases for the optional fourth argument — Go has no dynamic branch to be
// in parity with, so these live beside the corpus loop rather than inside it, and the corpus JSON
// itself is untouched (D2 property 3).
describe('http/substitute.ts resolve() dynamic callback (P6 D2/D3)', () => {
  test('two occurrences of one name produce two different values', () => {
    let n = 0;
    const result = resolve('{{$guid}}/{{$guid}}', {}, [], () => String(n++));
    expect(result.text).toBe('0/1');
    expect(result.refs).toEqual([
      { name: '$guid', kind: 'resolved' },
      { name: '$guid', kind: 'resolved' },
    ]);
  });

  test('a callback returning null leaves today’s behaviour exactly', () => {
    const withNullCallback = resolve('{{$nope}}', {}, [], () => null);
    const withNoCallback = resolve('{{$nope}}', {}, []);
    expect(withNullCallback).toEqual(withNoCallback);
    expect(withNullCallback.text).toBe('{{$nope}}');
    expect(withNullCallback.refs).toEqual([{ name: '$nope', kind: 'dynamic' }]);
  });

  test('a dynamic reference adjacent to a variable, a secret and an unknown resolve independently', () => {
    const result = resolve(
      '{{$guid}} {{apiKey}} {{token}} {{missing}}',
      { apiKey: 'abc123' },
      ['token'],
      () => 'GENERATED',
    );
    expect(result.text).toBe('GENERATED abc123 {{token}} {{missing}}');
    expect(result.refs).toEqual([
      { name: '$guid', kind: 'resolved' },
      { name: 'apiKey', kind: 'resolved' },
      { name: 'token', kind: 'deferred' },
      { name: 'missing', kind: 'unknown' },
    ]);
  });
});

// A template-reference span left literal because it names nothing
// (`unknown`) or generates nothing (`dynamic`, no catalogued generator) must not survive into the
// final URL carrying a raw space/&/#/= — those break url.Parse's RawQuery or the request line
// itself. `sanitizeUrlSpan` (the optional 5th argument) is applied to exactly those two kinds and
// never to `deferred` — a secret span a later, Go-side pass still has to find byte-identical.
describe('http/substitute.ts resolve() sanitizeUrlSpan (P14 round-2 finding 6)', () => {
  test('an unresolved {{base url}}-shaped reference in a query value survives as safe, parseable text', () => {
    const result = resolve(
      'https://api.example.com/orders?a={{base url}}&b=2',
      {},
      [],
      undefined,
      sanitizeUrlSpan,
    );
    expect(result.text).toBe('https://api.example.com/orders?a={{base%20url}}&b=2');
    expect(result.refs).toEqual([{ name: 'base url', kind: 'unknown' }]);
    // Proves the fix's own stated bar: the resulting URL actually parses, and the query string it
    // parses into is well-formed — the second, unrelated param survives untouched.
    const parsed = new URL(result.text);
    expect(parsed.searchParams.get('b')).toBe('2');
  });

  test('a deferred secret reference is never sanitized — a later pass still needs its exact name', () => {
    const result = resolve(
      '{{secret with spaces}}',
      {},
      ['secret with spaces'],
      undefined,
      sanitizeUrlSpan,
    );
    expect(result.text).toBe('{{secret with spaces}}');
    expect(result.refs).toEqual([{ name: 'secret with spaces', kind: 'deferred' }]);
  });

  test('a resolved value and ordinary literal text are unaffected', () => {
    const result = resolve('{{host}}/a b', { host: 'x.test' }, [], undefined, sanitizeUrlSpan);
    expect(result.text).toBe('x.test/a b');
  });

  test('an uncatalogued dynamic reference is sanitized the same way as unknown', () => {
    const result = resolve('{{$weird generator}}', {}, [], () => null, sanitizeUrlSpan);
    expect(result.text).toBe('{{$weird%20generator}}');
    expect(result.refs).toEqual([{ name: '$weird generator', kind: 'dynamic' }]);
  });
});

// classifyReference is resolve()'s own branch order, extracted so the highlighter and the
// hover agree with the send path about what each reference name means — this pins the precedence
// order itself ($dynamic before secret before value before unknown), independent of resolve()'s
// own corpus-driven cases above.
describe('http/substitute.ts classifyReference precedence (P15b D1)', () => {
  test('a $-prefixed name is dynamic even when it also happens to be a known value or secret', () => {
    expect(classifyReference('$guid', { $guid: 'x' }, ['$guid'])).toBe('dynamic');
    expect(classifyReference('$guid', {}, [])).toBe('dynamic');
  });

  test('a secret name is deferred even when the same name is also present in values', () => {
    expect(classifyReference('token', { token: 'leaked?' }, ['token'])).toBe('deferred');
  });

  test('a name present in values (and not a secret) is resolved', () => {
    expect(classifyReference('host', { host: 'api.example.com' }, [])).toBe('resolved');
  });

  test('a name in neither values nor secretNames is unknown', () => {
    expect(classifyReference('nope', { host: 'x' }, ['token'])).toBe('unknown');
  });
});

// P15b D1: splitTemplateSpans' offsets are the seam the colouring/hover work is built on — every
// span's `text` must equal `input.slice(from, to)`, which is the invariant every consumer (the
// range-highlight plugin, the hover lookup) depends on to paint or query the right characters.
describe('http/substitute.ts splitTemplateSpans offsets (P15b D1)', () => {
  function assertOffsetsMatch(input: string): void {
    for (const span of splitTemplateSpans(input)) {
      expect(input.slice(span.from, span.to)).toBe(span.text);
    }
  }

  test('adjacent references carry contiguous, non-overlapping offsets', () => {
    const input = '{{a}}{{b}}';
    assertOffsetsMatch(input);
    const spans = splitTemplateSpans(input);
    expect(spans).toEqual([
      { text: '{{a}}', isReference: true, from: 0, to: 5, name: 'a' },
      { text: '{{b}}', isReference: true, from: 5, to: 10, name: 'b' },
    ]);
  });

  test('an unterminated {{ is one trailing literal span with no name', () => {
    const input = 'before {{unterminated';
    assertOffsetsMatch(input);
    expect(splitTemplateSpans(input)).toEqual([
      { text: input, isReference: false, from: 0, to: input.length, name: '' },
    ]);
  });

  test('an empty {{}} is a non-reference span with an empty name', () => {
    const input = 'x{{}}y';
    assertOffsetsMatch(input);
    const spans = splitTemplateSpans(input);
    expect(spans[1]).toEqual({ text: '{{}}', isReference: false, from: 1, to: 5, name: '' });
  });

  test('the grammar’s own documented nesting oddity: {{a{{b}}}} takes "a{{b" as the name', () => {
    const input = '{{a{{b}}}}';
    assertOffsetsMatch(input);
    const spans = splitTemplateSpans(input);
    // find `{{` at 0, find the next `}}` at 7 — the name is `a{{b`, then `}}` (two chars) remains
    // as a trailing literal.
    expect(spans).toEqual([
      { text: '{{a{{b}}', isReference: true, from: 0, to: 8, name: 'a{{b' },
      { text: '}}', isReference: false, from: 8, to: 10, name: '' },
    ]);
  });
});

// A pipe applies AFTER per-occurrence dynamic-value generation, not before. The one assertion that
// pins it: two occurrences of the same `{{$guid | upper}}`-shaped reference must generate two
// *different* values (the per-occurrence freshness) and each must be independently transformed —
// never the same value generated once and reused.
describe('http/substitute.ts resolve() pipe/dynamic-value ordering (P17 D8)', () => {
  test('two occurrences of {{$guid | upper}} yield two different upper-cased values', () => {
    let n = 0;
    const result = resolve('{{$guid | upper}}/{{$guid | upper}}', {}, [], () => `id-${n++}`);
    expect(result.text).toBe('ID-0/ID-1');
    expect(result.text).not.toBe('ID-0/ID-0');
    expect(result.refs).toEqual([
      { name: '$guid', kind: 'resolved', pipeline: ['upper'] },
      { name: '$guid', kind: 'resolved', pipeline: ['upper'] },
    ]);
  });
});

// P17 D3: parseReference's own rule table, independent of resolve()'s corpus-driven cases above.
describe('http/substitute.ts parseReference (P17 D3)', () => {
  test('no pipe at all: the whole input is the name', () => {
    expect(parseReference('token')).toEqual({
      name: 'token',
      pipeline: [],
      normalized: '{{token}}',
    });
  });

  test('one recognised transform', () => {
    expect(parseReference('token | upper')).toEqual({
      name: 'token',
      pipeline: ['upper'],
      normalized: '{{token | upper}}',
    });
  });

  test('three chained recognised transforms', () => {
    expect(parseReference('token | base64 | upper | lower')).toEqual({
      name: 'token',
      pipeline: ['base64', 'upper', 'lower'],
      normalized: '{{token | base64 | upper | lower}}',
    });
  });

  test('an unrecognised segment falls back to treating the whole input as the name', () => {
    expect(parseReference('token | base46')).toEqual({
      name: 'token | base46',
      pipeline: [],
      normalized: '{{token | base46}}',
    });
  });

  test('an empty name before the pipe falls back to the whole input as the name', () => {
    expect(parseReference('| upper')).toEqual({
      name: '| upper',
      pipeline: [],
      normalized: '{{| upper}}',
    });
  });

  test('a whitespace-only segment is not a recognised transform, so the whole input is the name', () => {
    expect(parseReference('token |   ')).toEqual({
      name: 'token |   ',
      pipeline: [],
      normalized: '{{token |   }}',
    });
  });

  test('normalized is stable across spacing variants — the property D9’s masking placeholder depends on', () => {
    // Three spellings of the same span (differing only in whitespace around the name and the
    // pipe) must produce the exact same normalized placeholder, so two spellings of one span do
    // not produce two placeholders for identical wire bytes.
    const variants = ['token|upper', 'token | upper', 'token   |   upper'].map(
      (v) => parseReference(v).normalized,
    );
    expect(new Set(variants).size).toBe(1);
    expect(variants[0]).toBe('{{token | upper}}');
  });
});

// P17 D7: each transform's own round trip and its failure mode, plus the two byte-level traps the
// table calls out by name — bare `btoa` throwing on a non-ASCII value, and base64-valid-but-
// invalid-UTF-8 input to base64decode.
describe('http/substitute.ts transforms (P17 D7)', () => {
  test('base64 round-trips through base64decode', () => {
    expect(applyPipeline(['base64'], 'abc123')).toBe('YWJjMTIz');
    expect(applyPipeline(['base64', 'base64decode'], 'abc123')).toBe('abc123');
  });

  test('base64 over a non-ASCII value does not throw the way bare btoa would', () => {
    // '日' (U+65E5) has a code point > 255 — bare `btoa('日')` throws a DOMException/RangeError.
    // Routing through TextEncoder first (D7) is what makes this agree with Go's
    // base64.StdEncoding.EncodeToString([]byte(s)), which happily encodes the UTF-8 bytes.
    expect(applyPipeline(['base64'], '日')).toBe('5pel');
    expect(applyPipeline(['base64', 'base64decode'], '日')).toBe('日');
  });

  test('base64decode fails (not throws) on invalid base64', () => {
    expect(applyPipeline(['base64decode'], '!!!not-base64!!!')).toBeNull();
  });

  test('base64decode fails on valid base64 that decodes to invalid UTF-8', () => {
    // "/w==" is valid base64 for the single byte 0xFF, which is not valid UTF-8 on its own —
    // TextDecoder('utf-8', {fatal: true}) is what makes this a failure (D5), matching Go's
    // separate utf8.Valid check after a successful base64 decode.
    expect(applyPipeline(['base64decode'], '/w==')).toBeNull();
  });

  test('upper/lower round-trip and never fail', () => {
    expect(applyPipeline(['upper'], 'aB3')).toBe('AB3');
    expect(applyPipeline(['lower'], 'aB3')).toBe('ab3');
  });

  test('urlencode/urldecode round-trip, space as +', () => {
    expect(applyPipeline(['urlencode'], 'a b+c')).toBe('a+b%2Bc');
    expect(applyPipeline(['urlencode', 'urldecode'], 'a b+c')).toBe('a b+c');
  });

  test('urldecode fails on a malformed % escape', () => {
    expect(applyPipeline(['urldecode'], '%zz')).toBeNull();
  });

  test('an empty pipeline returns the value unchanged', () => {
    expect(applyPipeline([], 'unchanged')).toBe('unchanged');
  });
});

// P17 D11 (finding 4 extended): `|` joins the URL-unsafe set alongside space/&/#/= — a pipeline
// that never resolves (an unknown name piped through a real transform name) must not put a raw
// `|` into a request line either.
describe('http/substitute.ts sanitizeUrlSpan pipe handling (P17 D11)', () => {
  test('an unresolved piped reference in a URL is sanitized, pipe included, and the URL still parses', () => {
    const result = resolve(
      'https://api.example.com/orders?a={{missing | upper}}&b=2',
      {},
      [],
      undefined,
      sanitizeUrlSpan,
    );
    expect(result.text).toBe('https://api.example.com/orders?a={{missing%20%7C%20upper}}&b=2');
    expect(result.refs).toEqual([{ name: 'missing', kind: 'unknown', pipeline: ['upper'] }]);
    const parsed = new URL(result.text);
    expect(parsed.searchParams.get('b')).toBe('2');
  });
});
