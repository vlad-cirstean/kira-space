import type { MessageKey } from '@theme/i18n';
// beautify.ts's JSON scanner and ejson.ts's Mongo-shell-literal scanner each hand-
// rolled the same object/array node shape, the same object/array parse loop (open bracket,
// empty-container check, key/value/comma loop, close bracket) and the same indented/compact
// renderer. Shell differs only in tolerating a trailing comma and normalizing an unquoted/
// single-quoted key through JSON.stringify — both captured as the grammar/keyText the caller
// supplies. Both scanners stay spelling-preserving raw-slice re-encoders (CLAUDE.md's own named
// exception to the library rule) — this only collapses the two hand-rolled copies into one.

export type RawNode =
  | { kind: 'literal'; raw: string }
  | { kind: 'object'; members: { keyRaw: string; value: RawNode }[] }
  | { kind: 'array'; items: RawNode[] };

// P115 H9: beautify.ts's own Cursor and ejson.ts's own ShellCursor were the identical `{ text, i }`
// shape; isJsonWs/isShellWs and skipJsonWs/skipShellWs were byte-for-byte copies over it. One
// cursor shape and one whitespace skipper, shared.
export interface Cursor {
  text: string;
  i: number;
}

function isWs(c: string | undefined): boolean {
  return c === ' ' || c === '\t' || c === '\n' || c === '\r';
}

export function skipWs(c: Cursor): void {
  while (isWs(c.text[c.i])) c.i++;
}

export interface ContainerGrammar<C extends { text: string; i: number }> {
  /** Present for an object container, absent for an array — parseContainer reads which shape to
   *  parse (and which bracket pair) from this alone. Owns its own leading whitespace skip. */
  parseKey?: (c: C) => string;
  parseValue: (c: C) => RawNode;
  skipWs: (c: C) => void;
  allowTrailingComma: boolean;
  error: (offset: number) => Error;
}

/** After one member/item, consumes its trailing separator: `,` (continuing the loop, and — when
 *  `allowTrailingComma` — closing right there if a close bracket follows), the close bracket
 *  itself (`done: true`), or neither (a scan error). Shared by the object and array loops below,
 *  which differ only in what they parse before calling this. */
function consumeSeparator<C extends { text: string; i: number }>(
  c: C,
  close: string,
  grammar: Pick<ContainerGrammar<C>, 'skipWs' | 'allowTrailingComma' | 'error'>,
): { done: boolean } {
  grammar.skipWs(c);
  if (c.text[c.i] === ',') {
    c.i++;
    if (grammar.allowTrailingComma) {
      grammar.skipWs(c);
      if (c.text[c.i] === close) {
        c.i++;
        return { done: true };
      }
    }
    return { done: false };
  }
  if (c.text[c.i] === close) {
    c.i++;
    return { done: true };
  }
  throw grammar.error(c.i);
}

function parseObjectContainer<C extends { text: string; i: number }>(
  c: C,
  parseKey: (c: C) => string,
  grammar: ContainerGrammar<C>,
): RawNode {
  const members: { keyRaw: string; value: RawNode }[] = [];
  if (c.text[c.i] === '}') {
    c.i++;
    return { kind: 'object', members };
  }
  for (;;) {
    const keyRaw = parseKey(c);
    grammar.skipWs(c);
    if (c.text[c.i] !== ':') throw grammar.error(c.i);
    c.i++;
    members.push({ keyRaw, value: grammar.parseValue(c) });
    if (consumeSeparator(c, '}', grammar).done) break;
  }
  return { kind: 'object', members };
}

function parseArrayContainer<C extends { text: string; i: number }>(
  c: C,
  grammar: ContainerGrammar<C>,
): RawNode {
  const items: RawNode[] = [];
  if (c.text[c.i] === ']') {
    c.i++;
    return { kind: 'array', items };
  }
  for (;;) {
    items.push(grammar.parseValue(c));
    if (consumeSeparator(c, ']', grammar).done) break;
  }
  return { kind: 'array', items };
}

/** Parses an object (`grammar.parseKey` present) or array (absent) from `c`, positioned at its
 *  opening bracket. Mirrors beautify.ts's parseJsonObject/parseJsonArray and ejson.ts's
 *  parseShellObject/parseShellArray exactly, generalized over each grammar's own key/value/error
 *  primitives. */
export function parseContainer<C extends { text: string; i: number }>(
  c: C,
  grammar: ContainerGrammar<C>,
): RawNode {
  c.i++; // opening bracket
  grammar.skipWs(c);
  return grammar.parseKey
    ? parseObjectContainer(c, grammar.parseKey, grammar)
    : parseArrayContainer(c, grammar);
}

/** Reindented, one member/item per line. `keyText` formats an object member's raw key text —
 *  verbatim for JSON, JSON.stringify-normalized for the shell grammar. */
function renderIndented(node: RawNode, keyText: (raw: string) => string): string {
  const out: string[] = [];
  build(node, 0);
  return out.join('');

  function build(n: RawNode, depth: number): void {
    const pad = '  '.repeat(depth);
    const padIn = '  '.repeat(depth + 1);
    if (n.kind === 'literal') {
      out.push(n.raw);
      return;
    }
    if (n.kind === 'object') {
      if (n.members.length === 0) {
        out.push('{}');
        return;
      }
      out.push('{\n');
      n.members.forEach((m, idx) => {
        out.push(padIn, keyText(m.keyRaw), ': ');
        build(m.value, depth + 1);
        if (idx < n.members.length - 1) out.push(',');
        out.push('\n');
      });
      out.push(pad, '}');
      return;
    }
    if (n.items.length === 0) {
      out.push('[]');
      return;
    }
    out.push('[\n');
    n.items.forEach((item, idx) => {
      out.push(padIn);
      build(item, depth + 1);
      if (idx < n.items.length - 1) out.push(',');
      out.push('\n');
    });
    out.push(pad, ']');
  }
}

/** Single line, no whitespace. Same `keyText` contract as renderIndented. */
function renderCompact(node: RawNode, keyText: (raw: string) => string): string {
  const out: string[] = [];
  build(node);
  return out.join('');

  function build(n: RawNode): void {
    if (n.kind === 'literal') {
      out.push(n.raw);
      return;
    }
    if (n.kind === 'object') {
      out.push('{');
      n.members.forEach((m, idx) => {
        if (idx > 0) out.push(',');
        out.push(keyText(m.keyRaw), ':');
        build(m.value);
      });
      out.push('}');
      return;
    }
    out.push('[');
    n.items.forEach((item, idx) => {
      if (idx > 0) out.push(',');
      build(item);
    });
    out.push(']');
  }
}

// ---------------------------------------------------------------------------------------------
// beautify.ts's tryParseJson/beautifyJson and ejson.ts's tryParseShellText/
// beautifyShellText were the same two functions with a different grammar/error-class/keyText
// slotted in.
// ---------------------------------------------------------------------------------------------

export type ParseResult = { ok: true; node: RawNode } | { ok: false; offset: number };

/** Parses `text` in full: one value via `parseValue`, trailing whitespace via `skipWs`, then
 *  requires every remaining character consumed — anything left over is `new ErrorClass(offset)` at
 *  that final position, the same "trailing content" error `parseValue`'s own grammar throws for a
 *  mid-parse defect. Catches `ErrorClass` and stack-overflow `RangeError` only (never a stray bug
 *  in `parseValue`, which still throws through). */
export function tryParse<C extends Cursor, E extends Error & { offset: number }>(
  text: string,
  parseValue: (c: C) => RawNode,
  skipWsFn: (c: C) => void,
  ErrorClass: new (offset: number) => E,
): ParseResult {
  const c = { text, i: 0 } as C;
  try {
    const node = parseValue(c);
    skipWsFn(c);
    if (c.i !== text.length) throw new ErrorClass(c.i);
    return { ok: true, node };
  } catch (err) {
    if (err instanceof ErrorClass) return { ok: false, offset: err.offset };
    // Recursive descent overflows the stack on extreme nesting: same refusal as a syntax error.
    if (err instanceof RangeError) return { ok: false, offset: c.i };
    throw err;
  }
}

export type BeautifyMode = 'indented' | 'compact';

/** A failure message as catalog key plus params: text is resolved on the main thread, so a
 *  worker never loads the catalog runtime. */
export interface CatalogReason {
  key: MessageKey;
  params?: Record<string, string | number>;
}

export interface BeautifyResult {
  /** The reformatted text, or the input unchanged when `ok` is false. */
  text: string;
  ok: boolean;
  /** Present only when `ok` is false ('Invalid JSON at offset 4021.'). Shown on the status line. */
  reason?: CatalogReason;
}

/** Beautify/Minify for a `tryParse`-shaped scanner: parse, then reindent (`indented`) or flatten
 *  (`compact`) the result with `keyText`. `errorKind` picks the one thing beautify.ts's and
 *  ejson.ts's own error message differed on — invalid JSON vs invalid document text. */
export function beautifyWith(
  text: string,
  mode: BeautifyMode,
  tryParseText: (text: string) => ParseResult,
  errorKind: 'json' | 'document',
  keyText: (raw: string) => string,
): BeautifyResult {
  const r = tryParseText(text);
  if (!r.ok) {
    const key =
      errorKind === 'json'
        ? 'studio.beautify.invalidJson.error'
        : 'studio.beautify.invalidDocument.error';
    return { text, ok: false, reason: { key, params: { offset: r.offset } } };
  }
  try {
    const rendered =
      mode === 'indented' ? renderIndented(r.node, keyText) : renderCompact(r.node, keyText);
    return { text: rendered, ok: true };
  } catch (err) {
    if (err instanceof RangeError)
      return { text, ok: false, reason: { key: 'studio.beautify.nestedTooDeep.error' } };
    throw err;
  }
}
