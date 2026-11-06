import {
  type BeautifyMode,
  type BeautifyResult,
  beautifyWith,
  type CatalogReason,
  type Cursor,
  type ParseResult,
  parseContainer,
  type RawNode,
  skipWs,
  tryParse,
} from './views/shared/document/rawTree';

// P115 H9: BeautifyMode/BeautifyResult moved to rawTree.ts (this app's own JSON scanner and
// ejson.ts's shell-literal scanner both need them) — re-exported so every existing `from
// './beautify'`/`from '../beautify'` import of either keeps working unchanged.
export type { BeautifyMode, BeautifyResult };

// ---------------------------------------------------------------------------------------------
// JSON — a lossless scanner. Never JSON.parse/JSON.stringify: a number is reproduced from
// its exact raw slice, never round-tripped through a JS `number`, so a numeric(20,6)-shaped
// literal survives byte-identical. Structural whitespace is the only thing that changes.
// ---------------------------------------------------------------------------------------------

class JsonScanError extends Error {
  constructor(readonly offset: number) {
    super(`invalid JSON at offset ${offset}`);
  }
}

const JSON_NUMBER_RE = /-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?/y;

function parseJsonString(c: Cursor): string {
  const start = c.i;
  if (c.text[c.i] !== '"') throw new JsonScanError(c.i);
  c.i++;
  for (;;) {
    const ch = c.text[c.i];
    if (ch === undefined) throw new JsonScanError(c.i);
    if (ch === '"') {
      c.i++;
      return c.text.slice(start, c.i);
    }
    if (ch === '\\') {
      const esc = c.text[c.i + 1];
      if (esc === undefined) throw new JsonScanError(c.i);
      if (esc === 'u') {
        for (let k = 0; k < 4; k++) {
          const hex = c.text[c.i + 2 + k];
          if (hex === undefined || !/[0-9a-fA-F]/.test(hex)) throw new JsonScanError(c.i);
        }
        c.i += 6;
      } else if ('"\\/bfnrt'.includes(esc)) {
        c.i += 2;
      } else {
        throw new JsonScanError(c.i);
      }
      continue;
    }
    // JSON forbids raw (unescaped) control characters inside a string.
    if (ch.charCodeAt(0) <= 0x1f) throw new JsonScanError(c.i);
    c.i++;
  }
}

function parseJsonNumber(c: Cursor): string {
  JSON_NUMBER_RE.lastIndex = c.i;
  const m = JSON_NUMBER_RE.exec(c.text);
  if (!m || m.index !== c.i || m[0].length === 0) throw new JsonScanError(c.i);
  c.i += m[0].length;
  return m[0];
}

function parseJsonValue(c: Cursor): RawNode {
  skipWs(c);
  const ch = c.text[c.i];
  if (ch === '{') return parseJsonObject(c);
  if (ch === '[') return parseJsonArray(c);
  if (ch === '"') return { kind: 'literal', raw: parseJsonString(c) };
  if (ch === '-' || (ch >= '0' && ch <= '9')) return { kind: 'literal', raw: parseJsonNumber(c) };
  if (c.text.startsWith('true', c.i)) {
    c.i += 4;
    return { kind: 'literal', raw: 'true' };
  }
  if (c.text.startsWith('false', c.i)) {
    c.i += 5;
    return { kind: 'literal', raw: 'false' };
  }
  if (c.text.startsWith('null', c.i)) {
    c.i += 4;
    return { kind: 'literal', raw: 'null' };
  }
  throw new JsonScanError(c.i);
}

function parseJsonObject(c: Cursor): RawNode {
  return parseContainer(c, {
    parseKey: (cur) => {
      skipWs(cur);
      if (cur.text[cur.i] !== '"') throw new JsonScanError(cur.i);
      return parseJsonString(cur);
    },
    parseValue: parseJsonValue,
    skipWs,
    allowTrailingComma: false,
    error: (offset) => new JsonScanError(offset),
  });
}

function parseJsonArray(c: Cursor): RawNode {
  return parseContainer(c, {
    parseValue: parseJsonValue,
    skipWs,
    allowTrailingComma: false,
    error: (offset) => new JsonScanError(offset),
  });
}

function tryParseJson(text: string): ParseResult {
  return tryParse(text, parseJsonValue, skipWs, JsonScanError);
}

/** Used by detect.ts's `json` gate (§5b) — the one definition of "is this JSON" in the app. */
export function scanJson(text: string): { ok: boolean; offset?: number } {
  const r = tryParseJson(text);
  return r.ok ? { ok: true } : { ok: false, offset: r.offset };
}

// JSON keys are already double-quoted raw slices — rendered verbatim, no normalization.
const jsonKeyText = (raw: string): string => raw;

export function beautifyJson(text: string, mode: BeautifyMode): BeautifyResult {
  return beautifyWith(text, mode, tryParseJson, 'json', jsonKeyText);
}

// ---------------------------------------------------------------------------------------------
// XML / HTML — a lossless scanner. Attributes are copied verbatim inside their tag, never
// re-quoted/reordered/entity-normalised: only tag boundaries are found, never attribute contents.
// ---------------------------------------------------------------------------------------------

const VOID_ELEMENTS = new Set([
  'area',
  'base',
  'br',
  'col',
  'embed',
  'hr',
  'img',
  'input',
  'link',
  'meta',
  'source',
  'track',
  'wbr',
]);

class XmlScanError extends Error {
  constructor(readonly reason: CatalogReason) {
    super(reason.key);
  }
}

type XmlToken =
  | { kind: 'pi' | 'comment' | 'cdata' | 'doctype' | 'text'; raw: string; offset: number }
  | { kind: 'open'; raw: string; name: string; selfClosing: boolean; offset: number }
  | { kind: 'close'; raw: string; name: string; offset: number };

/** One scan's result: the token it read, and the index just past it — every `scanXml*` below
 *  shares this shape (`tokenizeXml` dispatches, one scanner per construct owns its own loop and
 *  its own error). Offsets and error strings are asserted byte-identical against the pre-split
 *  version. */
interface XmlScanStep {
  token: XmlToken;
  next: number;
}

function scanXmlTextRun(text: string, i: number, n: number): XmlScanStep {
  const start = i;
  while (i < n && text[i] !== '<') i++;
  return { token: { kind: 'text', raw: text.slice(start, i), offset: start }, next: i };
}

function scanXmlComment(text: string, i: number, start: number): XmlScanStep {
  const end = text.indexOf('-->', i + 4);
  if (end < 0)
    throw new XmlScanError({
      key: 'studio.beautify.unterminatedComment.error',
      params: { offset: start },
    });
  return { token: { kind: 'comment', raw: text.slice(i, end + 3), offset: start }, next: end + 3 };
}

function scanXmlCdata(text: string, i: number, start: number): XmlScanStep {
  const end = text.indexOf(']]>', i + 9);
  if (end < 0)
    throw new XmlScanError({
      key: 'studio.beautify.unterminatedSection.error',
      params: { keyword: 'CDATA', offset: start },
    });
  return { token: { kind: 'cdata', raw: text.slice(i, end + 3), offset: start }, next: end + 3 };
}

function scanXmlPi(text: string, i: number, start: number): XmlScanStep {
  const end = text.indexOf('?>', i + 2);
  if (end < 0)
    throw new XmlScanError({
      key: 'studio.beautify.unterminatedInstruction.error',
      params: { offset: start },
    });
  return { token: { kind: 'pi', raw: text.slice(i, end + 2), offset: start }, next: end + 2 };
}

function scanXmlDoctype(text: string, i: number, start: number, n: number): XmlScanStep {
  let j = i + 9;
  let bracketDepth = 0;
  let closed = false;
  while (j < n) {
    const c = text[j];
    if (c === '[') bracketDepth++;
    else if (c === ']') bracketDepth--;
    else if (c === '>' && bracketDepth <= 0) {
      j++;
      closed = true;
      break;
    }
    j++;
  }
  if (!closed) {
    throw new XmlScanError({
      key: 'studio.beautify.unterminatedDeclaration.error',
      params: { keyword: 'DOCTYPE', offset: start },
    });
  }
  return { token: { kind: 'doctype', raw: text.slice(start, j), offset: start }, next: j };
}

/** The attribute text after a tag's name, up to its unquoted `>`/`/>` — its own quote-state loop,
 *  split out of `scanXmlTag` so the name scan and this body scan each carry only their own
 *  complexity score. `closed` is false for either an unterminated tag or a quote still open at
 *  end of input — both are the same "unterminated tag" error to the caller. */
function scanXmlTagBody(
  text: string,
  j: number,
  n: number,
): { next: number; selfClosing: boolean; closed: boolean } {
  let quote: string | null = null;
  let selfClosing = false;
  let closedTag = false;
  while (j < n) {
    const c = text[j];
    if (quote) {
      if (c === quote) quote = null;
      j++;
      continue;
    }
    if (c === '"' || c === "'") {
      quote = c;
      j++;
      continue;
    }
    if (c === '/' && text[j + 1] === '>') {
      selfClosing = true;
      j += 2;
      closedTag = true;
      break;
    }
    if (c === '>') {
      j++;
      closedTag = true;
      break;
    }
    j++;
  }
  return { next: j, selfClosing, closed: closedTag && quote === null };
}

/** An opening or closing tag — name, then `scanXmlTagBody`'s attribute scan. */
function scanXmlTag(text: string, i: number, start: number, n: number): XmlScanStep {
  let j = i + 1;
  const closing = text[j] === '/';
  if (closing) j++;
  const nameStart = j;
  while (j < n && /[^\s/>]/.test(text[j])) j++;
  const name = text.slice(nameStart, j);
  if (name.length === 0)
    throw new XmlScanError({
      key: 'studio.beautify.malformedTag.error',
      params: { offset: start },
    });

  const body = scanXmlTagBody(text, j, n);
  if (!body.closed)
    throw new XmlScanError({
      key: 'studio.beautify.unterminatedTag.error',
      params: { offset: start },
    });
  const raw = text.slice(start, body.next);
  if (closing) {
    if (body.selfClosing)
      throw new XmlScanError({
        key: 'studio.beautify.malformedClosingTag.error',
        params: { offset: start },
      });
    return { token: { kind: 'close', raw, name, offset: start }, next: body.next };
  }
  return {
    token: { kind: 'open', raw, name, selfClosing: body.selfClosing, offset: start },
    next: body.next,
  };
}

function tokenizeXml(text: string): XmlToken[] {
  const tokens: XmlToken[] = [];
  const n = text.length;
  let i = 0;
  while (i < n) {
    if (text[i] !== '<') {
      const step = scanXmlTextRun(text, i, n);
      tokens.push(step.token);
      i = step.next;
      continue;
    }
    const start = i;
    let step: XmlScanStep;
    if (text.startsWith('<!--', i)) {
      step = scanXmlComment(text, i, start);
    } else if (text.startsWith('<![CDATA[', i)) {
      step = scanXmlCdata(text, i, start);
    } else if (text.startsWith('<?', i)) {
      step = scanXmlPi(text, i, start);
    } else if (/^<!doctype/i.test(text.slice(i, i + 9))) {
      step = scanXmlDoctype(text, i, start, n);
    } else {
      step = scanXmlTag(text, i, start, n);
    }
    tokens.push(step.token);
    i = step.next;
  }
  return tokens;
}

interface XmlElement {
  type: 'element';
  open: string;
  name: string;
  children: XmlNode[];
  close: string | null;
  void: boolean;
}
interface XmlLeaf {
  type: 'pi' | 'comment' | 'cdata' | 'doctype' | 'text';
  raw: string;
}
type XmlNode = XmlElement | XmlLeaf;

function buildXmlTree(tokens: XmlToken[], sourceLength: number): XmlNode[] {
  const root: XmlNode[] = [];
  const stack: { name: string; el: XmlElement | null; children: XmlNode[] }[] = [
    { name: '', el: null, children: root },
  ];
  for (const t of tokens) {
    const top = stack[stack.length - 1];
    if (t.kind === 'open') {
      const isVoid = VOID_ELEMENTS.has(t.name.toLowerCase());
      const el: XmlElement = {
        type: 'element',
        open: t.raw,
        name: t.name,
        children: [],
        close: null,
        void: t.selfClosing || isVoid,
      };
      top.children.push(el);
      if (!(t.selfClosing || isVoid)) stack.push({ name: t.name, el, children: el.children });
    } else if (t.kind === 'close') {
      if (stack.length <= 1) {
        throw new XmlScanError({
          key: 'studio.beautify.unmatchedClose.error',
          params: { tag: `</${t.name}>`, offset: t.offset },
        });
      }
      const entry = stack[stack.length - 1];
      if (entry.name !== t.name) {
        throw new XmlScanError({
          key: 'studio.beautify.mismatchedClose.error',
          params: { expected: `<${entry.name}>`, found: `</${t.name}>`, offset: t.offset },
        });
      }
      if (entry.el) entry.el.close = t.raw;
      stack.pop();
    } else {
      top.children.push({ type: t.kind, raw: t.raw });
    }
  }
  if (stack.length !== 1) {
    throw new XmlScanError({
      key: 'studio.beautify.unclosedTag.error',
      params: { tag: `<${stack[stack.length - 1].name}>`, offset: sourceLength },
    });
  }
  return root;
}

function tryParseXml(
  text: string,
): { ok: true; nodes: XmlNode[] } | { ok: false; reason: CatalogReason } {
  try {
    const tokens = tokenizeXml(text);
    const nodes = buildXmlTree(tokens, text.length);
    return { ok: true, nodes };
  } catch (err) {
    if (err instanceof XmlScanError) return { ok: false, reason: err.reason };
    if (err instanceof RangeError)
      return { ok: false, reason: { key: 'studio.beautify.nestedTooDeep.error' } };
    throw err;
  }
}

/** Used by detect.ts's `xml` gate (§5b): true only when every tag balances. */
export function scanXml(text: string): { ok: boolean } {
  return { ok: tryParseXml(text).ok };
}

function isWhitespaceOnlyXmlText(s: string): boolean {
  return /^[ \t\r\n]*$/.test(s);
}

function collectXmlIndentedLines(nodes: XmlNode[], depth: number, lines: string[]): void {
  const pad = '  '.repeat(depth);
  for (const node of nodes) {
    if (node.type === 'text') {
      if (isWhitespaceOnlyXmlText(node.raw)) continue;
      lines.push(pad + node.raw.trim());
      continue;
    }
    if (node.type === 'element') {
      lines.push(pad + node.open);
      if (!node.void) {
        collectXmlIndentedLines(node.children, depth + 1, lines);
        lines.push(pad + (node.close ?? ''));
      }
      continue;
    }
    lines.push(pad + node.raw);
  }
}

function renderXmlCompact(nodes: XmlNode[], out: string[]): void {
  for (const node of nodes) {
    if (node.type === 'text') {
      if (isWhitespaceOnlyXmlText(node.raw)) continue;
      out.push(node.raw);
      continue;
    }
    if (node.type === 'element') {
      out.push(node.open);
      if (!node.void) {
        renderXmlCompact(node.children, out);
        out.push(node.close ?? '');
      }
      continue;
    }
    out.push(node.raw);
  }
}

export function beautifyXml(text: string, mode: BeautifyMode): BeautifyResult {
  const r = tryParseXml(text);
  if (!r.ok) return { text, ok: false, reason: r.reason };
  try {
    if (mode === 'indented') {
      const lines: string[] = [];
      collectXmlIndentedLines(r.nodes, 0, lines);
      return { text: lines.join('\n'), ok: true };
    }
    const out: string[] = [];
    renderXmlCompact(r.nodes, out);
    return { text: out.join(''), ok: true };
  } catch (err) {
    if (err instanceof RangeError)
      return { text, ok: false, reason: { key: 'studio.beautify.nestedTooDeep.error' } };
    throw err;
  }
}
