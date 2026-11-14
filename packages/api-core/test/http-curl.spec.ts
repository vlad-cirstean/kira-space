// What earns a dedicated test is the interacting-rules half of this phase — mode selection
// × the effective Content-Type, method inference × -G × -T × a body flag, @/< handling × the flag
// it appears in, flag arity × an unknown flag × the URL pick, and the tokenizer's operator stop.
// One JSON corpus (curl-cases.json) covers tokenize/parseCurl, the same shape the substitution
// corpus established — minus a second language, since there is no Go twin to be in parity with,
// so it lives beside its one reader rather than in internal/*/testdata/.

import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { resolve as resolvePath } from 'node:path';
import type { HttpBodyWire, HttpHeaderWire, HttpRequestTabState } from '@kira/shared/domain/http';
import type { CurlRequest } from '../src/http/curl/generate';
import { toCurl } from '../src/http/curl/generate';
import { parseCurl } from '../src/http/curl/parse';
import type { CurlWarningKind } from '../src/http/curl/tokenize';
import { tokenize } from '../src/http/curl/tokenize';

interface TokenizeCase {
  name: string;
  command: string;
  argv?: string[];
  warnings?: CurlWarningKind[];
  error?: string;
}

interface ParseCase {
  name: string;
  command: string;
  want: Partial<HttpRequestTabState>;
  warnings: CurlWarningKind[];
}

const corpus: { tokenize: TokenizeCase[]; parse: ParseCase[] } = JSON.parse(
  readFileSync(resolvePath(import.meta.dir, 'curl-cases.json'), 'utf8'),
);

// Every field parseCurl's ParsedCurl.state always carries — a corpus case's `want` only states
// what differs from this, kept in one place so a new case does not have to restate the other nine.
const DEFAULT_STATE = {
  method: 'GET',
  url: '',
  headers: [] as unknown[],
  bodyMode: 'none',
  body: '',
  code: '',
  codeLanguage: 'json',
  urlEncoded: [] as unknown[],
  formData: [] as unknown[],
  binaryFile: null as unknown,
};

describe('http/curl/tokenize.ts (P7 D3)', () => {
  test('the corpus is non-empty', () => {
    expect(corpus.tokenize.length).toBeGreaterThan(0);
  });

  for (const c of corpus.tokenize) {
    test(c.name, () => {
      const result = tokenize(c.command);
      if (c.error !== undefined) {
        expect(result.ok).toBe(false);
        expect(result.ok ? '' : result.error).toBe(c.error);
        return;
      }
      expect(result.ok).toBe(true);
      if (!result.ok) return;
      expect(result.argv).toEqual(c.argv ?? []);
      expect(result.warnings.map((w) => w.kind)).toEqual(c.warnings ?? []);
    });
  }
});

describe('http/curl/parse.ts — parseCurl (P7 D4-D9)', () => {
  test('the corpus is non-empty', () => {
    expect(corpus.parse.length).toBeGreaterThan(0);
  });

  for (const c of corpus.parse) {
    test(c.name, () => {
      const result = parseCurl(c.command);
      expect('error' in result).toBe(false);
      if ('error' in result) return;
      expect(result.state).toEqual({ ...DEFAULT_STATE, ...c.want } as HttpRequestTabState);
      expect(result.warnings.map((w) => w.kind)).toEqual(c.warnings);
    });
  }
});

// ---- toCurl (P7 D13-D16) — not JSON-shaped (a CurlRequest's HttpBodyWire is naturally a TS
// object literal), each asserting one specific, measured rule rather than a whole command string,
// which would be brittle against a harmless reordering. ----

const EMPTY_BODY: HttpBodyWire = {
  mode: 'none',
  raw: '',
  code: '',
  codeLanguage: '',
  urlEncoded: [],
  formData: [],
  file: '',
};

function req(overrides: Partial<CurlRequest>): CurlRequest {
  return {
    method: 'GET',
    url: 'https://x.test/a',
    headers: [],
    body: EMPTY_BODY,
    defaultContentType: '',
    ...overrides,
  };
}

describe('http/curl/generate.ts — toCurl (P7 D13-D16)', () => {
  test('a GET with no body needs no -X, but always gets -L', () => {
    const command = toCurl(req({}));
    expect(command).toContain('-L');
    expect(command).not.toContain('-X');
  });

  test('a body flag makes -X unnecessary for POST, but DELETE with no body still needs it', () => {
    const withBody = toCurl(
      req({ method: 'POST', body: { ...EMPTY_BODY, mode: 'raw', raw: 'hi' } }),
    );
    expect(withBody).not.toContain('-X');
    expect(withBody).toContain('--data-raw');

    const noBody = toCurl(req({ method: 'DELETE' }));
    expect(noBody).toContain('-X');
    expect(noBody).toContain('DELETE');
  });

  test('the mode default Content-Type is emitted only when the user set none (D14)', () => {
    const auto = toCurl(
      req({
        method: 'POST',
        body: { ...EMPTY_BODY, mode: 'raw', raw: 'x' },
        defaultContentType: 'text/plain',
      }),
    );
    expect(auto).toContain('Content-Type: text/plain');

    const userSet: HttpHeaderWire[] = [{ name: 'Content-Type', value: 'application/xml' }];
    const explicit = toCurl(
      req({
        method: 'POST',
        headers: userSet,
        body: { ...EMPTY_BODY, mode: 'raw', raw: 'x' },
        defaultContentType: 'text/plain',
      }),
    );
    expect(explicit).toContain('application/xml');
    expect(explicit).not.toContain('text/plain');
  });

  test('a binary body removes the header curl would otherwise add (F11/D14)', () => {
    const command = toCurl(
      req({ method: 'POST', body: { ...EMPTY_BODY, mode: 'file', file: '/tmp/blob.bin' } }),
    );
    expect(command).toContain('-H Content-Type:');
    expect(command).toContain('--data-binary');
    expect(command).toContain('/tmp/blob.bin');
  });

  test('a formdata text row uses --form-string, never -F, even for a value starting with @ (F10)', () => {
    const command = toCurl(
      req({
        method: 'POST',
        body: {
          ...EMPTY_BODY,
          mode: 'formdata',
          formData: [{ name: 'note', kind: 'text', value: '@notafile', path: '', contentType: '' }],
        },
      }),
    );
    expect(command).toContain('--form-string');
    expect(command).not.toContain("-F 'note");
  });

  // internal/httpclient's formPartHeader sets a per-part Content-Type for a *text* form-data row
  // whenever the row carries one, but toCurl used to emit every text row as --form-string
  // unconditionally — which has no `;type=` syntax at all — so the generated command silently
  // stopped being equivalent to what Send sends. This fails against the pre-fix generator
  // (no 'type=' anywhere in the command) and passes once a text row's own Content-Type is emitted
  // via -F, mirroring the file row's own already-correct case just below.
  test('a formdata text row with its own Content-Type uses -F with ;type=, not --form-string (finding 5)', () => {
    const command = toCurl(
      req({
        method: 'POST',
        body: {
          ...EMPTY_BODY,
          mode: 'formdata',
          formData: [
            {
              name: 'metadata',
              kind: 'text',
              value: '{"a":1}',
              path: '',
              contentType: 'application/json',
            },
          ],
        },
      }),
    );
    expect(command).toContain('-F \'metadata={"a":1};type=application/json\'');
    expect(command).not.toContain('--form-string');
  });

  test('a formdata text row with a Content-Type but a leading @ still falls back to --form-string (F10 takes precedence)', () => {
    const command = toCurl(
      req({
        method: 'POST',
        body: {
          ...EMPTY_BODY,
          mode: 'formdata',
          formData: [
            { name: 'note', kind: 'text', value: '@notafile', path: '', contentType: 'text/plain' },
          ],
        },
      }),
    );
    expect(command).toContain('--form-string');
    expect(command).not.toContain("-F 'note");
  });

  // `;` is -F's own parameter separator, and the guard above
  // only checked the value's *first* character (the `@`/`<` case). A text value containing a
  // `;` — "Hello; world", say — emitted as `-F 'note=Hello; world;type=text/plain'` sends `note` =
  // "Hello" (curl reads ` world` as an unrecognised parameter, not part of the value): the
  // generated command silently stopped being equivalent to what Send puts on the wire, the
  // exact failure class this branch was introduced to fix, reopened for a value
  // containing `;` instead of a leading `@`/`<`. Falls back to --form-string, same tradeoff (the
  // type is lost) as the leading-`@`/`<` case just above.
  test('a formdata text row whose value contains a ";" falls back to --form-string, not a broken -F', () => {
    const command = toCurl(
      req({
        method: 'POST',
        body: {
          ...EMPTY_BODY,
          mode: 'formdata',
          formData: [
            {
              name: 'note',
              kind: 'text',
              value: 'Hello; world',
              path: '',
              contentType: 'text/plain',
            },
          ],
        },
      }),
    );
    expect(command).toContain('--form-string');
    expect(command).not.toContain("-F 'note");

    // The generated command must round-trip to the exact value Send would have sent — not a
    // truncated "Hello".
    const reparsed = parseCurl(command);
    if ('error' in reparsed) throw new Error(`parseCurl: ${reparsed.error}`);
    expect(reparsed.state.formData).toEqual([
      expect.objectContaining({ name: 'note', kind: 'text', value: 'Hello; world' }),
    ]);
  });

  test('a formdata file row uses -F, with a stated Content-Type appended (F10)', () => {
    const command = toCurl(
      req({
        method: 'POST',
        body: {
          ...EMPTY_BODY,
          mode: 'formdata',
          formData: [
            { name: 'file', kind: 'file', value: '', path: '/tmp/r.csv', contentType: 'text/csv' },
          ],
        },
      }),
    );
    expect(command).toContain("-F 'file=@/tmp/r.csv;type=text/csv'");
  });

  test("a user's own multipart Content-Type header is dropped — curl mints its own boundary (D15)", () => {
    const headers: HttpHeaderWire[] = [
      { name: 'Content-Type', value: 'multipart/form-data; boundary=abc' },
    ];
    const command = toCurl(
      req({
        method: 'POST',
        headers,
        body: {
          ...EMPTY_BODY,
          mode: 'formdata',
          formData: [{ name: 'a', kind: 'text', value: 'b', path: '', contentType: '' }],
        },
      }),
    );
    expect(command).not.toContain('boundary=abc');
  });

  test('urlencoded emits the pre-encoded string as --data-raw, matching Go byte-for-byte (F13)', () => {
    const command = toCurl(
      req({
        method: 'POST',
        body: {
          ...EMPTY_BODY,
          mode: 'urlencoded',
          urlEncoded: [
            { name: 'a b', value: 'c d' },
            { name: 'x&y', value: 'z&w' },
          ],
        },
      }),
    );
    expect(command).not.toContain('--data-urlencode');
    expect(command).toContain("--data-raw 'a+b=c+d&x%26y=z%26w'");
  });

  test('every argument is shell-quoted (D13) — a single quote in a body round-trips through shlex', () => {
    const command = toCurl(
      req({ method: 'POST', body: { ...EMPTY_BODY, mode: 'raw', raw: "it's a test" } }),
    );
    expect(command).toContain(`'it'"'"'s a test'`);
  });

  test('an unresolved {{token}} is emitted verbatim, never a reveal by itself (D10)', () => {
    const headers: HttpHeaderWire[] = [{ name: 'Authorization', value: 'Bearer {{token}}' }];
    const command = toCurl(req({ headers }));
    expect(command).toContain('{{token}}');
  });

  test('an Authorization header is emitted as a header, never decoded back into -u (D16)', () => {
    const headers: HttpHeaderWire[] = [{ name: 'Authorization', value: 'Basic YWxpY2U6czNjcjN0' }];
    const command = toCurl(req({ headers }));
    expect(command).toContain('-H');
    expect(command).toContain('Authorization: Basic YWxpY2U6czNjcjN0');
    expect(command).not.toContain('-u ');
  });
});

// ---- The round trip (D17 item 3) — the strongest single property this phase can assert: it
// catches an escaping bug and a mapping bug in one assertion, in both directions at once. ----

describe('parseCurl(toCurl(x)) round-trips every mode (P7 D17)', () => {
  function roundTrip(request: CurlRequest): ReturnType<typeof parseCurl> {
    return parseCurl(toCurl(request));
  }

  test('none', () => {
    const result = roundTrip(req({ method: 'GET' }));
    expect('error' in result).toBe(false);
    if ('error' in result) return;
    expect(result.state.method).toBe('GET');
    expect(result.state.url).toBe('https://x.test/a');
    expect(result.state.bodyMode).toBe('none');
  });

  test('raw, with an embedded quote, newline and backslash', () => {
    const raw = "line one\nit's a \\backslash\\ test";
    const result = roundTrip(
      req({
        method: 'POST',
        body: { ...EMPTY_BODY, mode: 'raw', raw },
        defaultContentType: 'text/plain',
      }),
    );
    expect('error' in result).toBe(false);
    if ('error' in result) return;
    expect(result.state.bodyMode).toBe('raw');
    expect(result.state.body).toBe(raw);
  });

  for (const codeLanguage of ['javascript', 'json', 'html', 'xml'] as const) {
    test(`code · ${codeLanguage}`, () => {
      const code = `<content for ${codeLanguage}> & "quoted" 'text'`;
      const result = roundTrip(
        req({
          method: 'POST',
          body: { ...EMPTY_BODY, mode: 'code', code, codeLanguage },
          defaultContentType:
            codeLanguage === 'json'
              ? 'application/json'
              : codeLanguage === 'xml'
                ? 'application/xml'
                : codeLanguage === 'html'
                  ? 'text/html'
                  : 'application/javascript',
        }),
      );
      expect('error' in result).toBe(false);
      if ('error' in result) return;
      expect(result.state.bodyMode).toBe('code');
      expect(result.state.codeLanguage).toBe(codeLanguage);
      expect(result.state.code).toBe(code);
    });
  }

  test('urlencoded, with &, =, +, a space and a % in a value', () => {
    const result = roundTrip(
      req({
        method: 'POST',
        body: {
          ...EMPTY_BODY,
          mode: 'urlencoded',
          urlEncoded: [{ name: 'q', value: 'a&b=c+d e%f' }],
        },
        defaultContentType: 'application/x-www-form-urlencoded',
      }),
    );
    expect('error' in result).toBe(false);
    if ('error' in result) return;
    expect(result.state.bodyMode).toBe('urlencoded');
    expect(result.state.urlEncoded).toEqual([
      { name: 'q', value: 'a&b=c+d e%f', enabled: true, description: '' },
    ]);
  });

  // toCurl joins a multi-row urlencoded body into exactly one
  // --data-raw with its rows '&'-joined — the same shape every browser's "Copy as cURL"
  // produces for a form POST. parseAsKeyValue used to split that one piece at only the *first*
  // '=' and never on '&', collapsing every field after the first into the first row's own value.
  test('urlencoded, with two or more fields, round-trips as separate rows (not collapsed into one)', () => {
    const result = roundTrip(
      req({
        method: 'POST',
        body: {
          ...EMPTY_BODY,
          mode: 'urlencoded',
          urlEncoded: [
            { name: 'username', value: 'alice' },
            { name: 'password', value: 'hunter2' },
            { name: 'remember', value: '1' },
          ],
        },
        defaultContentType: 'application/x-www-form-urlencoded',
      }),
    );
    expect('error' in result).toBe(false);
    if ('error' in result) return;
    expect(result.state.bodyMode).toBe('urlencoded');
    expect(result.state.urlEncoded).toEqual([
      { name: 'username', value: 'alice', enabled: true, description: '' },
      { name: 'password', value: 'hunter2', enabled: true, description: '' },
      { name: 'remember', value: '1', enabled: true, description: '' },
    ]);
  });

  test('formdata — a text row, a file row, and a text value starting with @', () => {
    const result = roundTrip(
      req({
        method: 'POST',
        body: {
          ...EMPTY_BODY,
          mode: 'formdata',
          formData: [
            { name: 'title', kind: 'text', value: 'hello world', path: '', contentType: '' },
            { name: 'note', kind: 'text', value: '@notafile', path: '', contentType: '' },
            { name: 'file', kind: 'file', value: '', path: '/tmp/r.csv', contentType: 'text/csv' },
          ],
        },
      }),
    );
    expect('error' in result).toBe(false);
    if ('error' in result) return;
    expect(result.state.bodyMode).toBe('formdata');
    expect(result.state.formData).toEqual([
      {
        name: 'title',
        kind: 'text',
        value: 'hello world',
        path: '',
        fileName: '',
        fileSize: 0,
        contentType: '',
        enabled: true,
        description: '',
      },
      {
        name: 'note',
        kind: 'text',
        value: '@notafile',
        path: '',
        fileName: '',
        fileSize: 0,
        contentType: '',
        enabled: true,
        description: '',
      },
      // A re-parsed curl command never carries a live path — toCurl(x) is plain text once
      // generated, indistinguishable from a pasted command an attacker wrote, so re-importing it
      // never trusts an `@path` any more than Postman's own import does. Path is unresolved;
      // only the display name survives.
      {
        name: 'file',
        kind: 'file',
        value: '',
        path: '',
        fileName: 'r.csv',
        fileSize: 0,
        contentType: 'text/csv',
        enabled: true,
        description: '',
      },
    ]);
  });

  // Before the fix, a text row's own Content-Type never reached
  // toCurl's output at all, so this round trip would come back with contentType: '' — silently
  // losing the field. Now that toCurl emits it via -F's own `;type=`, and parse.ts already reads
  // that syntax (the note: "the import half *does* read `;type=`"), the value survives
  // the whole Copy-as-curl -> Import-curl round trip.
  test('formdata — a text row with its own Content-Type survives the round trip (finding 5)', () => {
    const result = roundTrip(
      req({
        method: 'POST',
        body: {
          ...EMPTY_BODY,
          mode: 'formdata',
          formData: [
            {
              name: 'metadata',
              kind: 'text',
              value: '{"a":1}',
              path: '',
              contentType: 'application/json',
            },
          ],
        },
      }),
    );
    expect('error' in result).toBe(false);
    if ('error' in result) return;
    expect(result.state.formData).toEqual([
      {
        name: 'metadata',
        kind: 'text',
        value: '{"a":1}',
        path: '',
        fileName: '',
        fileSize: 0,
        contentType: 'application/json',
        enabled: true,
        description: '',
      },
    ]);
  });

  test('file (binary)', () => {
    const result = roundTrip(
      req({ method: 'POST', body: { ...EMPTY_BODY, mode: 'file', file: '/tmp/blob.bin' } }),
    );
    expect('error' in result).toBe(false);
    if ('error' in result) return;
    expect(result.state.bodyMode).toBe('file');
    // Same reasoning as the formdata file row above: a re-parsed curl command never carries a
    // live path forward.
    expect(result.state.binaryFile).toEqual({ path: '', name: 'blob.bin', size: 0 });
  });
});

// ---- J-HTTP-062 / 064: bytes measured from a real curl, and the errors a broken paste gives ----

describe('http/curl/parse.ts — -G against real curl (OQ-API-3)', () => {
  const recorded: {
    curl: string;
    cases: { name: string; command: string; requestTarget: string }[];
  } = JSON.parse(readFileSync(resolvePath(import.meta.dir, 'testdata/curl-g-wire.json'), 'utf8'));

  for (const c of recorded.cases) {
    test(`${c.name}: the query equals the one ${recorded.curl} sent`, () => {
      const parsed = parseCurl(c.command);
      expect('error' in parsed).toBe(false);
      if ('error' in parsed) return;
      const url = new URL(parsed.state.url);
      expect(`${url.pathname}${url.search}`).toBe(c.requestTarget);
    });
  }
});

describe('http/curl/parse.ts — broken pastes (J-HTTP-064)', () => {
  test('an unterminated quote is an error, not a half-parsed request', () => {
    expect('error' in parseCurl("curl 'https://h")).toBe(true);
  });

  test('two URLs report how many were dropped', () => {
    const parsed = parseCurl('curl https://a https://b');
    if ('error' in parsed) throw new Error(parsed.error);
    expect(parsed.state.url).toBe('https://a');
    expect(parsed.warnings.map((w) => [w.kind, w.detail.params?.count])).toEqual([
      ['multiple-urls', 1],
    ]);
  });
});

describe('http/curl/parse.ts — flags that change nothing (J-HTTP-063)', () => {
  // D6: -s, -S, -L, --compressed, -o, --retry never reach the server as a different request, so they
  // stay silent; an unknown flag warns by its raw text and never eats the URL.
  test('ignored flags are silent, the unknown flag warns by raw text', () => {
    const parsed = parseCurl('curl --retry 3 --my-flag https://h/x -sSL --compressed -o out.txt');
    if ('error' in parsed) throw new Error(parsed.error);
    expect(parsed.state.url).toBe('https://h/x');
    expect(parsed.warnings.map((w) => [w.kind, w.detail.params?.raw])).toEqual([
      ['unknown-flag', '--my-flag'],
    ]);
  });
});
