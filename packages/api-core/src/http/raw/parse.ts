import {
  HTTP_METHODS,
  type HttpCodeLanguage,
  type HttpHeaderState,
  type HttpRequestTabState,
} from '@kira/shared/domain/http';
import { t } from '@theme/i18n';
import type { TextRef } from '../../lib/textRef';
import { explicitContentType } from '../headers';

// P9 D11: the warning vocabulary — a closed union, mirroring http/curl/tokenize.ts's own
// CurlWarningKind shape (P7 D4) so the dialog can render either list with the same component.
const RAW_WARNING_KINDS = ['chunked-transfer-encoding', 'content-length-dropped'] as const;
type RawWarningKind = (typeof RAW_WARNING_KINDS)[number];
export interface RawWarning {
  kind: RawWarningKind;
  detail: TextRef;
}

export interface ParsedRawRequest {
  /** Only the fields raw HTTP text can express — urlEncoded/formData/binaryFile are untouched
   *  (a parsed body always lands in `raw` or `code`, never those three), so the caller's
   *  patch leaves whatever those buffers already held (the "every mode keeps its own
   *  buffer" rule, applied here too). */
  state: Pick<
    HttpRequestTabState,
    'method' | 'url' | 'headers' | 'bodyMode' | 'body' | 'code' | 'codeLanguage'
  >;
  warnings: RawWarning[];
}

/** The exact table: application/json → json; application/xml/text/xml → xml; text/html
 *  → html; application/javascript → javascript; everything else (including
 *  application/x-www-form-urlencoded) → null, which lands the body in `raw`. Deliberately a
 *  narrower table than http/curl/parse.ts's own codeLanguageForContentType (no `+json`/`+xml`
 *  suffixes, no `text/javascript`). */
function codeLanguageForContentType(contentType: string): HttpCodeLanguage | null {
  const type = contentType.split(';')[0].trim().toLowerCase();
  if (type === 'application/json') return 'json';
  if (type === 'application/xml' || type === 'text/xml') return 'xml';
  if (type === 'text/html') return 'html';
  if (type === 'application/javascript') return 'javascript';
  return null;
}

/** Splits on '\n', stripping one trailing '\r' per line — tolerant of both a generate.ts-produced
 *  buffer (\n) and a pasted one (\r\n); D11's grammar never depends on which line ending arrived. */
function splitLines(text: string): string[] {
  return text.split('\n').map((line) => (line.endsWith('\r') ? line.slice(0, -1) : line));
}

/** A leading-'/' target joins onto the tab's current origin (editing just the path); anything else
 *  replaces the URL outright (D11 item 2) — including a `{{base_url}}/...` reference, which is not
 *  a URI and must never go through `new URL()`. */
function resolveTarget(currentUrl: string, target: string): string {
  if (!target.startsWith('/')) return target;
  try {
    return new URL(currentUrl).origin + target;
  } catch {
    return target;
  }
}

/**
 * Raw HTTP/1.1 text → a tab state patch plus a warning list. Never throws — an unparseable
 * request comes back as `{error}`, mirroring the `ParsedCurl` shape so the dialog's
 * warning/error strip needs no change.
 *
 * 1. Request line: `METHOD SP target [SP HTTP-version]`. The version is optional and dropped — the
 *    app speaks whatever the transport negotiates, so accepting `HTTP/1.1` and then possibly
 *    sending h2 would be exactly the lie.
 * 2. The target is taken verbatim, `{{…}}` and all — no `new URL()`, no normalisation.
 * 3. Header lines until the first blank line, split at the first colon, value trimmed of one
 *    leading space. Name case and line order are preserved exactly; a duplicate name is a second
 *    row, never merged. An obs-fold continuation line (leading space/tab) is an error naming its
 *    line number rather than being silently joined.
 * 4. A `Host:` header is kept as an ordinary header row — never folded into the URL.
 * 5. The body is everything after the first blank line, verbatim. `Transfer-Encoding: chunked`
 *    warns rather than doing anything — buildBody always computes an exact Content-Length and
 *    never sends chunked.
 * 6. A `Content-Length` header is dropped with a note — Go computes the real one at send time.
 */
export function parseRawRequest(
  text: string,
  currentUrl: string,
): ParsedRawRequest | { error: string } {
  const lines = splitLines(text);

  const requestLine = lines[0] ?? '';
  const firstSpace = requestLine.indexOf(' ');
  if (firstSpace === -1) {
    return { error: t('studio.http.editRaw.parse.noSpace.error') };
  }
  const methodToken = requestLine.slice(0, firstSpace);
  if (!(HTTP_METHODS as readonly string[]).includes(methodToken)) {
    return {
      error: t('studio.http.editRaw.parse.unknownMethod.error', {
        method: methodToken,
        methods: HTTP_METHODS.join(', '),
      }),
    };
  }
  const method = methodToken as HttpRequestTabState['method'];

  const afterMethod = requestLine.slice(firstSpace + 1).trim();
  const versionMatch = afterMethod.match(/^(\S+)\s+HTTP\/\d(?:\.\d)?$/);
  const target = versionMatch ? versionMatch[1] : afterMethod;
  if (target === '') {
    return { error: t('studio.http.editRaw.parse.noTarget.error') };
  }
  const url = resolveTarget(currentUrl, target);

  const headers: HttpHeaderState[] = [];
  const warnings: RawWarning[] = [];
  let bodyStartLine = lines.length;
  for (let i = 1; i < lines.length; i++) {
    const line = lines[i];
    if (line === '') {
      bodyStartLine = i + 1;
      break;
    }
    if (line[0] === ' ' || line[0] === '\t') {
      return { error: t('studio.http.editRaw.parse.folded.error', { line: i + 1 }) };
    }
    const colon = line.indexOf(':');
    if (colon === -1) {
      return { error: t('studio.http.editRaw.parse.noColon.error', { line: i + 1, text: line }) };
    }
    const name = line.slice(0, colon);
    const value = line.slice(colon + 1).replace(/^ /, '');

    if (name.trim().toLowerCase() === 'content-length') {
      warnings.push({
        kind: 'content-length-dropped',
        detail: {
          key: 'workbench.apiImport.contentLengthDropped.body',
          params: { line, header: 'Content-Length' },
        },
      });
      continue;
    }
    if (
      name.trim().toLowerCase() === 'transfer-encoding' &&
      value.trim().toLowerCase() === 'chunked'
    ) {
      warnings.push({
        kind: 'chunked-transfer-encoding',
        detail: {
          key: 'workbench.apiImport.chunkedKept.body',
          params: { line, header: 'Content-Length' },
        },
      });
    }
    headers.push({ name, value, enabled: true, description: '' });
  }

  const body = bodyStartLine < lines.length ? lines.slice(bodyStartLine).join('\n') : '';

  let bodyMode: HttpRequestTabState['bodyMode'] = 'none';
  let bodyRaw = '';
  let bodyCode = '';
  let codeLanguage: HttpCodeLanguage = 'json';
  if (body !== '') {
    const contentType = explicitContentType(headers);
    const lang = contentType ? codeLanguageForContentType(contentType) : null;
    if (lang) {
      bodyMode = 'code';
      codeLanguage = lang;
      bodyCode = body;
    } else {
      bodyMode = 'raw';
      bodyRaw = body;
    }
  }

  return {
    state: { method, url, headers, bodyMode, body: bodyRaw, code: bodyCode, codeLanguage },
    warnings,
  };
}
