import {
  HTTP_METHODS,
  type HttpBinaryFileState,
  type HttpCodeLanguage,
  type HttpFormDataFieldState,
  type HttpHeaderState,
  type HttpRequestTabState,
  type HttpUrlEncodedFieldState,
} from '@kira/shared/domain/http';
import { explicitContentType } from '../headers';
import { type CurlFlagId, expandShortCluster, type FlagSpec, lookupFlag } from './flags';
import { type CurlWarning, tokenize } from './tokenize';

export interface ParsedCurl {
  /** Only the fields a curl command can express — everything else keeps its default (the caller
   *  patches a freshly opened tab, D12). */
  state: Pick<
    HttpRequestTabState,
    | 'method'
    | 'url'
    | 'headers'
    | 'bodyMode'
    | 'body'
    | 'code'
    | 'codeLanguage'
    | 'urlEncoded'
    | 'formData'
    | 'binaryFile'
  >;
  warnings: CurlWarning[];
}

interface RawDataPiece {
  id: 'data' | 'data-raw' | 'data-binary' | 'data-urlencode';
  text: string;
}

function basename(path: string): string {
  const parts = path.split('/');
  return parts[parts.length - 1] || path;
}

/** UTF-8-correct, never-throwing base64 encode. `btoa` alone only handles Latin-1 code points: it
 *  throws `InvalidCharacterError` outside that range (breaking `parseCurl`'s "never throws"
 *  contract) and, for the codepoints it does accept, encodes them as Latin-1 bytes rather than the
 *  UTF-8 bytes curl itself sends — wrong credentials, silently. Encoding to UTF-8 bytes first
 *  and feeding `btoa` each byte as its own Latin-1 code unit sidesteps both: every byte is < 256,
 *  so `btoa` never throws. */
function utf8Base64(s: string): string {
  const bytes = new TextEncoder().encode(s);
  let binary = '';
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

/** D7's "parses as k=v" test: a non-empty name before the first '='. Splitting happens on the raw
 *  (possibly percent-encoded) text — an encoded '=' would already read back as '%3D', never a
 *  literal '=', so the real separator is never ambiguous with an encoded one. */
function parseAsKeyValue(piece: string): { name: string; value: string } | null {
  const eq = piece.indexOf('=');
  if (eq <= 0) return null;
  return { name: piece.slice(0, eq), value: piece.slice(eq + 1) };
}

/** The inverse of generate.ts's own `goQueryEscape` — decodes a piece's name/value back to plain
 *  text before it lands in a `urlEncoded` row (the round trip requires this: `toCurl` emits the
 *  already-encoded string as `--data-raw`, so reconstructing the row has to undo that or a
 *  re-send would double-encode it). A malformed %-sequence falls back to the literal text rather
 *  than throwing — this is untrusted pasted input. */
function decodeQueryComponent(s: string): string {
  try {
    return decodeURIComponent(s.replace(/\+/g, ' '));
  } catch {
    return s;
  }
}

/** A `-d`/`--data-raw`/`--data-binary` piece is wire-ready text —
 *  this app's own `toCurl` joins a multi-row urlencoded body into exactly one `--data-raw` with
 *  its rows `&`-joined (generate.ts), and the browser "Copy as cURL" shape every form POST
 *  produces is the same single-flag, `&`-joined piece — so an unescaped `&` there does
 *  separate fields; splitting on it here is `toCurl`'s join, undone. `--data-urlencode`'s value is
 *  different: it is literal text curl itself percent-encodes before sending, so an `&` inside
 *  it is an ordinary character, never a field separator, and must not be split. */
function toUrlEncodedRows(pieces: readonly RawDataPiece[]): HttpUrlEncodedFieldState[] {
  const rows: HttpUrlEncodedFieldState[] = [];
  for (const piece of pieces) {
    // A `--data-urlencode` piece's value is literal text *curl
    // itself* percent-encodes before putting it on the wire (the comment above the `&`-split
    // already says so) — decodeQueryComponent below used to run on it anyway, so a piece curl
    // would send byte-for-byte (`q=a+b` -> `q=a%2Bb` on the wire) got decoded into `"a b"` and
    // re-sent as a *different* value (`q=a%20b`). `--data`/`--data-raw`/`--data-binary` pieces are
    // the opposite case (already percent-encoded text this app's own toCurl produced) and still
    // need decoding to avoid double-encoding on re-send (the round trip, described above).
    const isDataUrlencode = piece.id === 'data-urlencode';
    const subPieces = isDataUrlencode ? [piece.text] : piece.text.split('&');
    for (const sub of subPieces) {
      const kv = parseAsKeyValue(sub);
      rows.push({
        name: isDataUrlencode ? (kv?.name ?? '') : decodeQueryComponent(kv?.name ?? ''),
        value: isDataUrlencode ? (kv?.value ?? '') : decodeQueryComponent(kv?.value ?? ''),
        enabled: true,
        description: '',
      });
    }
  }
  return rows;
}

/** F11: which of this app's CODE_LANGUAGES a Content-Type's subtype maps onto, or null when the
 *  type belongs in `raw` instead (D7 step 1). */
function codeLanguageForContentType(contentType: string): HttpCodeLanguage | null {
  const type = contentType.split(';')[0].trim().toLowerCase();
  if (type === 'application/json' || type.endsWith('+json')) return 'json';
  if (type === 'application/xml' || type === 'text/xml' || type.endsWith('+xml')) return 'xml';
  if (type === 'text/html') return 'html';
  if (type === 'application/javascript' || type === 'text/javascript') return 'javascript';
  return null;
}

/** What curl puts on the query for `-G --data-urlencode content`, `=content` or `name=content`: the
 *  content percent-encoded, the name left as typed. Bytes measured from curl 8.5.0
 *  (test/testdata/curl-g-wire.json): space is `+`, hex is lowercase, and only `A-Za-z0-9-._~` pass. */
function encodeDataUrlencodePiece(text: string): string {
  const eq = text.indexOf('=');
  const content = eq === -1 ? text : text.slice(eq + 1);
  let encoded = '';
  for (const byte of new TextEncoder().encode(content)) {
    const ch = String.fromCharCode(byte);
    if (/[A-Za-z0-9\-._~]/.test(ch)) encoded += ch;
    else if (byte === 0x20) encoded += '+';
    else encoded += `%${byte.toString(16).padStart(2, '0')}`;
  }
  return eq > 0 ? `${text.slice(0, eq)}=${encoded}` : encoded;
}

/** D8: the three file-referencing --data-urlencode spellings — `@path`, `=@path`, `name@path`. */
function dataUrlencodeIsFile(value: string): boolean {
  if (value.startsWith('@') || value.startsWith('=@')) return true;
  const eq = value.indexOf('=');
  const at = value.indexOf('@');
  return at !== -1 && (eq === -1 || at < eq);
}

/** The argv-loop's shared mutable state, named once so `handle` and its two flag
 *  helpers can be lifted out of `parseCurl` as module-level functions over it —
 *  the point is that each helper's own cognitive-complexity score, and its nesting penalty inside
 *  `parseCurl`, both leave the parent rather than moving sideways. */
interface CurlAccumulator {
  warnings: CurlWarning[];
  requestFlagValue: string | undefined;
  headFlag: boolean;
  uploadFilePath: string | undefined;
  getFlag: boolean;
  urlFlagValue: string | undefined;
  headerRows: HttpHeaderState[];
  nonFlagArgs: string[];
  rawDataPieces: RawDataPiece[];
  formFields: HttpFormDataFieldState[];
  // D5's "any -d-family or -F flag -> POST" precedence has to see that -F was *used*, even for a
  // `-F 'k=<f'` piece D8 drops entirely (form-file-content) — formFields.length alone would miss
  // it once the row is gone.
  sawFormFlag: boolean;
}

function pushHeader(acc: CurlAccumulator, name: string, value: string): void {
  acc.headerRows.push({ name, value, enabled: true, description: '' });
}

function handleHeaderFlag(acc: CurlAccumulator, raw: string): void {
  const colon = raw.indexOf(':');
  if (colon !== -1) {
    const name = raw.slice(0, colon).trim();
    const value = raw.slice(colon + 1).replace(/^ /, '');
    if (name === '') {
      acc.warnings.push({
        kind: 'header-malformed',
        detail: { key: 'workbench.apiImport.headerNoName.body', params: { raw } },
      });
    } else {
      pushHeader(acc, name, value);
    }
    return;
  }
  // F12: curl's documented `Name;` form sends an empty-valued header.
  if (raw.endsWith(';')) {
    pushHeader(acc, raw.slice(0, -1).trim(), '');
    return;
  }
  acc.warnings.push({
    kind: 'header-malformed',
    detail: { key: 'workbench.apiImport.headerNoColon.body', params: { raw } },
  });
}

function handleFormFlag(acc: CurlAccumulator, id: 'form' | 'form-string', raw: string): void {
  const eq = raw.indexOf('=');
  if (eq === -1) {
    acc.formFields.push({
      name: raw,
      kind: 'text',
      value: '',
      path: '',
      fileName: '',
      fileSize: 0,
      contentType: '',
      enabled: true,
      description: '',
    });
    return;
  }
  const name = raw.slice(0, eq);
  const rest = raw.slice(eq + 1);

  // F10/D16: --form-string never gives '@'/'<'/';type='/';filename=' any special meaning — the
  // whole remainder is the literal text value, exactly what D15 generates it from.
  if (id === 'form-string') {
    acc.formFields.push({
      name,
      kind: 'text',
      value: rest,
      path: '',
      fileName: '',
      fileSize: 0,
      contentType: '',
      enabled: true,
      description: '',
    });
    return;
  }

  const segments = rest.split(';');
  const content = segments[0];
  let contentType = '';
  let hasFilenameOverride = false;
  for (const segment of segments.slice(1)) {
    const segEq = segment.indexOf('=');
    if (segEq === -1) continue;
    const key = segment.slice(0, segEq).trim();
    const value = segment.slice(segEq + 1);
    if (key === 'type') contentType = value;
    else if (key === 'filename') hasFilenameOverride = true;
  }

  if (content.startsWith('@')) {
    // P21 round 2 architecture/security finding 1: the sibling of round 1's Postman fix
    // (internal/postman/body.go's own WarnUnresolvedFile) — a pasted curl command is
    // attacker-controlled input exactly like an imported collection, so `path` is left
    // unresolved/empty rather than carried straight into a live, sendable field. Send's own
    // "no file chosen for form-data field" refusal then does the rest until the user re-picks
    // the file themselves through FilesService.ChooseOpen. The name is kept only for display.
    const path = content.slice(1);
    acc.warnings.push({
      kind: 'unresolved-file',
      detail: {
        key: 'workbench.apiImport.fileRef.body',
        params: { flag: '-F', value: `${name}=@${path}` },
      },
    });
    acc.formFields.push({
      name,
      kind: 'file',
      value: '',
      path: '',
      fileName: basename(path),
      fileSize: 0,
      contentType,
      enabled: true,
      description: '',
    });
  } else if (content.startsWith('<')) {
    acc.warnings.push({
      kind: 'form-file-content',
      detail: { key: 'workbench.apiImport.formFileContent.body', params: { raw } },
    });
    return;
  } else {
    acc.formFields.push({
      name,
      kind: 'text',
      value: content,
      path: '',
      fileName: '',
      fileSize: 0,
      contentType,
      enabled: true,
      description: '',
    });
  }
  if (hasFilenameOverride) {
    acc.warnings.push({
      kind: 'form-filename',
      detail: { key: 'workbench.apiImport.formFilename.body', params: { raw } },
    });
  }
}

function handle(
  acc: CurlAccumulator,
  spec: FlagSpec,
  raw: string,
  value: string | undefined,
): void {
  if (spec.category === 'ignored') return;
  if (spec.category === 'warned') {
    acc.warnings.push({
      kind: 'unsupported-flag',
      detail: { key: 'workbench.apiImport.unsupportedFlag.body', params: { raw } },
    });
    return;
  }
  const id = spec.id as CurlFlagId;
  const v = value ?? '';
  switch (id) {
    case 'request':
      acc.requestFlagValue = v;
      return;
    case 'head':
      acc.headFlag = true;
      return;
    case 'upload-file':
      acc.uploadFilePath = v;
      return;
    case 'get':
      acc.getFlag = true;
      return;
    case 'url':
      acc.urlFlagValue = v;
      return;
    case 'header':
      handleHeaderFlag(acc, v);
      return;
    case 'user-agent':
      pushHeader(acc, 'User-Agent', v);
      return;
    case 'referer':
      pushHeader(acc, 'Referer', v);
      return;
    case 'cookie':
      if (v.includes('=')) {
        pushHeader(acc, 'Cookie', v);
      } else {
        acc.warnings.push({
          kind: 'unsupported-flag',
          detail: {
            key: 'workbench.apiImport.cookieJar.body',
            params: { flag: '-b/--cookie', value: v },
          },
        });
      }
      return;
    case 'user':
      if (v.includes('{{')) {
        // Base64-encoding a value that still carries a {{variable}} reference would encode
        // the literal braces and destroy the reference — curl never sees {{...}}, so this is
        // this app's own template syntax pasted into -u's value, meaningful only unencoded. Keep
        // the header unencoded and warn instead of silently destroying the reference.
        pushHeader(acc, 'Authorization', `Basic ${v}`);
        acc.warnings.push({
          kind: 'credential-in-command',
          detail: {
            key: 'workbench.apiImport.userVariable.body',
            params: {
              flag: '-u/--user',
              value: v,
              reference: '{{variable}}',
              example: 'user:pass',
              pipe: '{{name | base64}}',
            },
          },
        });
        return;
      }
      pushHeader(acc, 'Authorization', `Basic ${utf8Base64(v)}`);
      acc.warnings.push({
        kind: 'credential-in-command',
        detail: { key: 'workbench.apiImport.credential.body', params: { flag: '-u/--user' } },
      });
      return;
    case 'oauth2-bearer':
      pushHeader(acc, 'Authorization', `Bearer ${v}`);
      acc.warnings.push({
        kind: 'credential-in-command',
        detail: {
          key: 'workbench.apiImport.credential.body',
          params: { flag: '--oauth2-bearer' },
        },
      });
      return;
    case 'json':
      // curl's --json is --data-binary plus these two headers — including --data-binary's
      // own `@file` file-reference handling (unlike --data-raw, which never treats a leading `@`
      // as a file). Pushing it under 'data-binary' routes an `@path` value through the same
      // unresolved-file/file-mode handling `--data-binary` already gets, instead of importing the
      // literal text `@body.json` as the body with no warning.
      pushHeader(acc, 'Content-Type', 'application/json');
      pushHeader(acc, 'Accept', 'application/json');
      acc.rawDataPieces.push({ id: 'data-binary', text: v });
      return;
    case 'data':
    case 'data-raw':
    case 'data-binary':
    case 'data-urlencode':
      acc.rawDataPieces.push({ id, text: v });
      return;
    case 'form':
    case 'form-string':
      acc.sawFormFlag = true;
      handleFormFlag(acc, id, v);
      return;
  }
}

/** A `--flag=value` token split into its bare flag and inline value — `--flag value` (two argv
 *  slots) has no inline value, `undefined` here. */
function splitInlineValue(token: string): { token: string; inlineValue: string | undefined } {
  if (!token.startsWith('--')) return { token, inlineValue: undefined };
  const eq = token.indexOf('=');
  if (eq === -1) return { token, inlineValue: undefined };
  return { token: token.slice(0, eq), inlineValue: token.slice(eq + 1) };
}

/** A short-cluster expansion (`-sL` → `-s -L`) or, failing that, D6's unknown-flag warning. Either
 *  way the token consumes exactly one argv slot — never a value, since neither path knows the
 *  flag's arity. Returns the next index. */
function handleUnrecognizedToken(
  acc: CurlAccumulator,
  token: string,
  raw: string,
  i: number,
): number {
  const cluster = !token.startsWith('--') ? expandShortCluster(token) : null;
  if (cluster) {
    for (const flag of cluster) {
      const clusterSpec = lookupFlag(flag);
      if (clusterSpec) handle(acc, clusterSpec, flag, undefined);
    }
    return i + 1;
  }
  // D6: an unknown flag is assumed to take no value — the alternative (assume it takes one)
  // eats the URL, F6's exact failure mode.
  acc.warnings.push({
    kind: 'unknown-flag',
    detail: { key: 'workbench.apiImport.unknownFlag.body', params: { raw } },
  });
  return i + 1;
}

/** One argv slot that is a flag (the non-flag case is handled by the caller). Returns the next
 *  index — 1 or 2 slots consumed, depending on the flag's arity and whether its value was
 *  inline (`--flag=value`) or in the next slot (`--flag value`). */
function consumeFlagToken(acc: CurlAccumulator, argv: readonly string[], i: number): number {
  const raw = argv[i];
  const { token, inlineValue } = splitInlineValue(raw);
  const spec = lookupFlag(token);
  if (!spec) return handleUnrecognizedToken(acc, token, raw, i);
  if (spec.arity === 0) {
    handle(acc, spec, raw, undefined);
    return i + 1;
  }
  if (inlineValue !== undefined) {
    handle(acc, spec, raw, inlineValue);
    return i + 1;
  }
  handle(acc, spec, raw, argv[i + 1]);
  return i + 2;
}

/** The argv loop, over the accumulator `handle` and its helpers mutate. */
function parseArgv(acc: CurlAccumulator, argv: readonly string[]): void {
  let i = 0;
  while (i < argv.length) {
    const raw = argv[i];
    if (raw.length === 0 || raw[0] !== '-' || raw === '-') {
      acc.nonFlagArgs.push(raw);
      i += 1;
      continue;
    }
    i = consumeFlagToken(acc, argv, i);
  }
}

/** The tail of the old `parseCurl` (P94 pass 3, §4.3): method + URL + body-mode resolution, once
 *  the argv loop has filled the accumulator. Split out of `parseCurl` itself so the loop and this
 *  decision phase each carry only their own cognitive-complexity score. */
/** D5's method precedence, `-X` > `-I`/`--head` > `-T`/`--upload-file` > `-G`/`--get` > any
 *  data/form flag > the GET default. */
function resolveMethod(acc: CurlAccumulator): string {
  if (acc.requestFlagValue !== undefined) {
    const upper = acc.requestFlagValue.toUpperCase();
    if ((HTTP_METHODS as readonly string[]).includes(upper)) return upper;
    acc.warnings.push({
      kind: 'method-coerced',
      detail: {
        key: 'workbench.apiImport.methodCoerced.body',
        params: { method: acc.requestFlagValue, fallback: 'GET' },
      },
    });
    return 'GET';
  }
  if (acc.headFlag) return 'HEAD';
  if (acc.uploadFilePath !== undefined) return 'PUT';
  if (acc.getFlag) return 'GET';
  if (acc.rawDataPieces.length > 0 || acc.sawFormFlag) return 'POST';
  return 'GET';
}

/** D5's URL pick (`--url`, else the first non-flag arg), the "extra URL-shaped argument" warning,
 *  and F13's `-G` query-string merge — the merge has to run here because it also empties
 *  `acc.rawDataPieces`, which the body-mode resolution below reads next. */
function resolveUrl(acc: CurlAccumulator): string {
  let url = acc.urlFlagValue ?? acc.nonFlagArgs[0] ?? '';
  const extraCount =
    acc.urlFlagValue !== undefined
      ? acc.nonFlagArgs.length
      : Math.max(0, acc.nonFlagArgs.length - 1);
  if (extraCount > 0) {
    acc.warnings.push({
      kind: 'multiple-urls',
      detail: { key: 'workbench.apiImport.multipleUrls.body', params: { count: extraCount } },
    });
  }
  if (url === '') {
    acc.warnings.push({ kind: 'no-url', detail: { key: 'workbench.apiImport.noUrl.body' } });
  }

  // F13: -G appends the -d-family data verbatim to the query string rather than sending it as a
  // body — measured: curl never re-encodes it, so neither does this. A `--data-urlencode` piece is
  // the exception: curl encodes its content before appending it (OQ-API-3). A piece that would read a
  // local file is dropped with a warning, as everywhere else.
  if (acc.getFlag && acc.rawDataPieces.length > 0) {
    const queryText = filterLiteralPieces(acc)
      .map((p) => (p.id === 'data-urlencode' ? encodeDataUrlencodePiece(p.text) : p.text))
      .join('&');
    acc.rawDataPieces.length = 0;
    if (queryText !== '') url += (url.includes('?') ? '&' : '?') + queryText;
  }
  return url;
}

interface BodyResolution {
  bodyMode: HttpRequestTabState['bodyMode'];
  bodyRaw: string;
  bodyCode: string;
  codeLanguage: HttpCodeLanguage;
  urlEncodedRows: HttpUrlEncodedFieldState[];
  binaryFile: HttpBinaryFileState;
}

function noBody(): BodyResolution {
  return {
    bodyMode: 'none',
    bodyRaw: '',
    bodyCode: '',
    codeLanguage: 'json',
    urlEncodedRows: [],
    binaryFile: null,
  };
}

/** Drops the three "would inline a local file's contents" piece shapes (D8), warning for each —
 *  the pieces that remain are what `decideBodyFromPieces` below actually renders as a body. */
function filterLiteralPieces(acc: CurlAccumulator): RawDataPiece[] {
  const literalPieces: RawDataPiece[] = [];
  for (const piece of acc.rawDataPieces) {
    if (piece.id === 'data' && piece.text.startsWith('@')) {
      acc.warnings.push({
        kind: 'data-file-inline',
        detail: {
          key: 'workbench.apiImport.fileInlineStripped.body',
          params: { flag: '-d', value: piece.text },
        },
      });
      continue;
    }
    if (piece.id === 'data-binary' && piece.text.startsWith('@')) {
      acc.warnings.push({
        kind: 'data-file-inline',
        detail: {
          key: 'workbench.apiImport.fileInline.body',
          params: { flag: '--data-binary', value: piece.text },
        },
      });
      continue;
    }
    if (piece.id === 'data-urlencode' && dataUrlencodeIsFile(piece.text)) {
      acc.warnings.push({
        kind: 'data-file-inline',
        detail: {
          key: 'workbench.apiImport.fileInline.body',
          params: { flag: '--data-urlencode', value: piece.text },
        },
      });
      continue;
    }
    literalPieces.push(piece);
  }
  return literalPieces;
}

/** D7's content-type/all-key-value decision tree, once file-referencing pieces are already
 *  filtered out. */
function decideBodyFromPieces(acc: CurlAccumulator, literalPieces: RawDataPiece[]): BodyResolution {
  if (literalPieces.length === 0) return noBody();

  const contentType = explicitContentType(acc.headerRows);
  const allKeyValue = literalPieces.every((p) => parseAsKeyValue(p.text) !== null);
  const joined = () => literalPieces.map((p) => p.text).join('&');

  if (contentType !== undefined) {
    if (
      contentType.split(';')[0].trim().toLowerCase() === 'application/x-www-form-urlencoded' &&
      allKeyValue
    ) {
      return {
        ...noBody(),
        bodyMode: 'urlencoded',
        urlEncodedRows: toUrlEncodedRows(literalPieces),
      };
    }
    const lang = codeLanguageForContentType(contentType);
    if (lang) return { ...noBody(), bodyMode: 'code', codeLanguage: lang, bodyCode: joined() };
    return { ...noBody(), bodyMode: 'raw', bodyRaw: joined() };
  }
  if (allKeyValue) {
    return { ...noBody(), bodyMode: 'urlencoded', urlEncodedRows: toUrlEncodedRows(literalPieces) };
  }
  // F11: no Content-Type header means curl would have sent this as urlencoded regardless — raw's
  // own default is text/plain (§1.6), so an explicit header is added and named.
  pushHeader(acc, 'Content-Type', 'application/x-www-form-urlencoded');
  acc.warnings.push({
    kind: 'implied-content-type',
    detail: {
      key: 'workbench.apiImport.impliedContentType.body',
      params: { type: 'application/x-www-form-urlencoded', header: 'Content-Type' },
    },
  });
  return { ...noBody(), bodyMode: 'raw', bodyRaw: joined() };
}

/** The body-mode dispatch: formdata wins outright, then a lone `--data-binary @path` is
 *  `file` mode, then any other data piece goes through `decideBodyFromPieces`, then a bare
 *  `-T`/`--upload-file` is `file` mode too. Must run after `resolveUrl`, whose `-G` merge can
 *  empty `acc.rawDataPieces` first. */
function resolveBodyMode(acc: CurlAccumulator): BodyResolution {
  // D8: `--data-binary @path` as the *only* data piece is "one local file as the whole body" —
  // exactly what `file` mode is.
  const onlyBinaryFile =
    acc.rawDataPieces.length === 1 &&
    acc.formFields.length === 0 &&
    acc.rawDataPieces[0].id === 'data-binary' &&
    acc.rawDataPieces[0].text.startsWith('@');

  if (acc.formFields.length > 0) {
    // D7 item 3: formdata wins regardless of any stated Content-Type header — curl mints its own
    // boundary, so a stated one cannot be honoured (dropped silently, the same way it would be
    // overridden on the real wire).
    return { ...noBody(), bodyMode: 'formdata' };
  }
  if (onlyBinaryFile) {
    // P21 round 2 architecture/security finding 1: same fix as the -F @path branch above — the
    // path is attacker-controlled text from the pasted command, not something this app resolved,
    // so it is never carried into a live, sendable field.
    const path = acc.rawDataPieces[0].text.slice(1);
    acc.warnings.push({
      kind: 'unresolved-file',
      detail: {
        key: 'workbench.apiImport.fileRef.body',
        params: { flag: '--data-binary', value: `@${path}` },
      },
    });
    return {
      ...noBody(),
      bodyMode: 'file',
      binaryFile: { path: '', name: basename(path), size: 0 },
    };
  }
  if (acc.rawDataPieces.length > 0) {
    return decideBodyFromPieces(acc, filterLiteralPieces(acc));
  }
  if (acc.uploadFilePath !== undefined) {
    // P21 round 2 architecture/security finding 1: same fix as above — -T/--upload-file's path
    // comes from the pasted command text, not a file this app resolved itself.
    acc.warnings.push({
      kind: 'unresolved-file',
      detail: {
        key: 'workbench.apiImport.fileRef.body',
        params: { flag: '-T', value: acc.uploadFilePath },
      },
    });
    return {
      ...noBody(),
      bodyMode: 'file',
      binaryFile: { path: '', name: basename(acc.uploadFilePath), size: 0 },
    };
  }
  return noBody();
}

/** The tail of the old `parseCurl` (P94 pass 3, §4.3): method + URL + body-mode resolution, once
 *  the argv loop has filled the accumulator. Split into `resolveMethod`/`resolveUrl`/
 *  `resolveBodyMode` so each decision phase carries only its own cognitive-complexity score. */
function resolveMethodAndBody(acc: CurlAccumulator): ParsedCurl {
  const method = resolveMethod(acc);
  const url = resolveUrl(acc);
  const body = resolveBodyMode(acc);

  return {
    state: {
      method: method as HttpRequestTabState['method'],
      url,
      headers: acc.headerRows,
      bodyMode: body.bodyMode,
      body: body.bodyMode === 'raw' ? body.bodyRaw : '',
      code: body.bodyMode === 'code' ? body.bodyCode : '',
      codeLanguage: body.codeLanguage,
      urlEncoded: body.bodyMode === 'urlencoded' ? body.urlEncodedRows : [],
      formData: body.bodyMode === 'formdata' ? acc.formFields : [],
      binaryFile: body.binaryFile,
    },
    warnings: acc.warnings,
  };
}

/**
 * D4-D9: turns pasted curl text into a state patch plus a warning list — never throws; an
 * unparseable command comes back as `{error}` (D3's tokenizer error, verbatim).
 */
export function parseCurl(text: string): ParsedCurl | { error: string } {
  const tokenized = tokenize(text);
  if (!tokenized.ok) return { error: tokenized.error };

  const acc: CurlAccumulator = {
    warnings: [...tokenized.warnings],
    requestFlagValue: undefined,
    headFlag: false,
    uploadFilePath: undefined,
    getFlag: false,
    urlFlagValue: undefined,
    headerRows: [],
    nonFlagArgs: [],
    rawDataPieces: [],
    formFields: [],
    sawFormFlag: false,
  };

  parseArgv(acc, tokenized.argv);
  return resolveMethodAndBody(acc);
}
