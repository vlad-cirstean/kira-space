import { t } from '@theme/i18n';
// Parses one document body — canonical extended JSON, `read.ts`'s `EJSON.stringify(doc, {relaxed:
// false})` — into a plain, frozen, non-reactive node tree, and formats values back out in Mongo
// shell form. No `bson` import: every BSON wrapper is recognised by shape against the closed EJSON
// v2 spec — the renderer never parses a wire protocol.
//
// A canonical numeric wrapper's own value is always a JSON *string* (`{"$numberLong":"123"}`), so
// `JSON.parse` never rounds a BSON integer through a lossy JS `number` the way it would for the SQL
// cell editor's raw numeric literals (`beautify.ts`'s reason for a hand-written scanner there) —
// plain `JSON.parse` is exact here.
import {
  type BeautifyMode,
  type BeautifyResult,
  beautifyWith,
  type Cursor,
  type ParseResult,
  parseContainer,
  type RawNode,
  skipWs,
  tryParse,
} from './rawTree';

type DocNodeKind = 'object' | 'array' | 'scalar';

/** The BSON type a node resolved to, for the type tooltip and the ObjectId/date affordances.
 *  'json' means a plain JSON scalar with no extended-JSON wrapper around it. */
export type BsonType =
  | 'json'
  | 'ObjectId'
  | 'Date'
  | 'Int32'
  | 'Int64'
  | 'Double'
  | 'Decimal128'
  | 'Binary'
  | 'Timestamp'
  | 'RegExp'
  | 'Code'
  | 'DBRef'
  | 'MinKey'
  | 'MaxKey'
  | 'UUID';

export interface DocNode {
  /** Field name, or the index as text inside an array. '' for the root. */
  key: string;
  /** Dotted path from the root ('device.os', 'tags.0'); '' for the root. Identity for the per-path
   *  expansion set (documentRows.ts D4) — stable across a re-parse of the same body. */
  path: string;
  kind: DocNodeKind;
  /** scalar only: the rendered text — `ObjectId("507f…")`, `"active"`, `148`, `null`. */
  text: string;
  /** scalar only: which --kira-syntax-* colour the text takes. */
  token: 'string' | 'number' | 'keyword' | 'bson';
  bsonType: BsonType;
  /** object/array only. Frozen. */
  children: readonly DocNode[];
  /** object/array only: '{…} 3 fields' / '[…] 12 items' — what a collapsed container shows. */
  summary: string;
}

interface ScalarRender {
  text: string;
  token: DocNode['token'];
  bsonType: BsonType;
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function objectKeys(value: Record<string, unknown>): string[] {
  return Object.keys(value);
}

// One canonical millis value out of a $date wrapper: usually a nested {"$numberLong": "..."}
// (what this app's own EJSON.stringify({relaxed:false}) always writes), tolerating a bare
// ISO/millis string in case a document was written by some other tool. BSON dates are int64
// millis but a JS Date holds only +-8.64e15; beyond that (a "never expires" sentinel) there is no
// Date to build, so null keeps the wrapper verbatim instead of throwing in toISOString.
const MAX_DATE_MILLIS = 8.64e15;
// Relaxed EJSON uses ISO strings only for years 1970-9999.
const MAX_RELAXED_DATE_MILLIS = 253_402_300_799_999;
function inDateRange(n: number): number | null {
  return Number.isFinite(n) && Math.abs(n) <= MAX_DATE_MILLIS ? n : null;
}
function dateMillis(value: unknown): number | null {
  if (
    isPlainObject(value) &&
    objectKeys(value).length === 1 &&
    typeof value.$numberLong === 'string'
  ) {
    return inDateRange(Number(value.$numberLong));
  }
  if (typeof value === 'string') return inDateRange(Date.parse(value));
  if (typeof value === 'number') return inDateRange(value);
  return null;
}

interface WrapperSpec {
  /** Whether `value`'s own keys match this wrapper's shape — key set and value types alike. */
  match(keys: readonly string[], value: Record<string, unknown>): boolean;
  /** `null` means "matched the key shape but not a renderable value" (only `$numberDouble`'s own
   *  Infinity/NaN case) — `detectWrapper`'s own loop then falls through to the next spec, which,
   *  since wrapper key shapes are mutually exclusive, means the overall `null` fallback. */
  render(value: Record<string, unknown>): ScalarRender | null;
}

// P94 pass 3 §4.3: one table entry per EJSON wrapper `detectWrapper` recognises — this file's own
// doc comment above (moved to `detectWrapper` itself) still names which types render as
// constructor calls vs. verbatim EJSON text.
const WRAPPER_TABLE: readonly WrapperSpec[] = [
  {
    match: (keys, value) =>
      keys.length === 1 && keys[0] === '$oid' && typeof value.$oid === 'string',
    render: (value) => ({ text: `ObjectId("${value.$oid}")`, token: 'bson', bsonType: 'ObjectId' }),
  },
  {
    match: (keys) => keys.length === 1 && keys[0] === '$date',
    render: (value) => {
      const millis = dateMillis(value.$date);
      if (millis === null) return null;
      return {
        text: `ISODate("${new Date(millis).toISOString()}")`,
        token: 'bson',
        bsonType: 'Date',
      };
    },
  },
  {
    match: (keys, value) =>
      keys.length === 1 && keys[0] === '$numberInt' && typeof value.$numberInt === 'string',
    render: (value) => ({
      text: `NumberInt(${value.$numberInt})`,
      token: 'bson',
      bsonType: 'Int32',
    }),
  },
  {
    match: (keys, value) =>
      keys.length === 1 && keys[0] === '$numberLong' && typeof value.$numberLong === 'string',
    render: (value) => ({
      text: `NumberLong("${value.$numberLong}")`,
      token: 'bson',
      bsonType: 'Int64',
    }),
  },
  {
    match: (keys, value) =>
      keys.length === 1 && keys[0] === '$numberDecimal' && typeof value.$numberDecimal === 'string',
    render: (value) => ({
      text: `NumberDecimal("${value.$numberDecimal}")`,
      token: 'bson',
      bsonType: 'Decimal128',
    }),
  },
  {
    match: (keys, value) =>
      keys.length === 1 && keys[0] === '$numberDouble' && typeof value.$numberDouble === 'string',
    render: (value) => {
      const raw = value.$numberDouble as string;
      // Infinity/-Infinity/NaN have no bare-number JSON spelling — fall through to the verbatim
      // fallback below rather than emitting text `parseDocumentLiteral` cannot re-tokenize.
      return Number.isFinite(Number(raw))
        ? { text: raw, token: 'number', bsonType: 'Double' }
        : null;
    },
  },
  {
    match: (keys, value) =>
      keys.length === 1 &&
      keys[0] === '$binary' &&
      isPlainObject(value.$binary) &&
      typeof value.$binary.base64 === 'string' &&
      typeof value.$binary.subType === 'string',
    render: (value) => {
      const subType = (value.$binary as { subType: string }).subType;
      return {
        text: JSON.stringify(value),
        token: 'bson',
        bsonType: subType.toLowerCase() === '04' ? 'UUID' : 'Binary',
      };
    },
  },
  {
    match: (keys, value) =>
      keys.length === 1 &&
      keys[0] === '$timestamp' &&
      isPlainObject(value.$timestamp) &&
      typeof value.$timestamp.t === 'number' &&
      typeof value.$timestamp.i === 'number',
    render: (value) => ({ text: JSON.stringify(value), token: 'bson', bsonType: 'Timestamp' }),
  },
  {
    match: (keys, value) =>
      keys.length === 1 &&
      keys[0] === '$regularExpression' &&
      isPlainObject(value.$regularExpression) &&
      typeof value.$regularExpression.pattern === 'string',
    render: (value) => ({ text: JSON.stringify(value), token: 'bson', bsonType: 'RegExp' }),
  },
  {
    match: (keys, value) =>
      (keys.length === 1 || keys.length === 2) &&
      keys.includes('$code') &&
      typeof value.$code === 'string' &&
      keys.every((k) => k === '$code' || k === '$scope'),
    render: (value) => ({ text: JSON.stringify(value), token: 'bson', bsonType: 'Code' }),
  },
  {
    match: (keys, value) =>
      keys.includes('$ref') &&
      keys.includes('$id') &&
      typeof value.$ref === 'string' &&
      keys.every((k) => k === '$ref' || k === '$id' || k === '$db'),
    render: (value) => ({ text: JSON.stringify(value), token: 'bson', bsonType: 'DBRef' }),
  },
  {
    match: (keys, value) => keys.length === 1 && keys[0] === '$minKey' && value.$minKey === 1,
    render: (value) => ({ text: JSON.stringify(value), token: 'bson', bsonType: 'MinKey' }),
  },
  {
    match: (keys, value) => keys.length === 1 && keys[0] === '$maxKey' && value.$maxKey === 1,
    render: (value) => ({ text: JSON.stringify(value), token: 'bson', bsonType: 'MaxKey' }),
  },
];

/**
 * Recognises a single-purpose EJSON wrapper object by shape and renders its shell text. Returns
 * `null` for a plain object (no known wrapper key). The six types `literal.ts` can construct
 * (ObjectId, ISODate/Date, NumberLong, NumberInt, NumberDecimal) render as constructor calls;
 * Double renders as a bare number, since typing a bare number into the shell already produces one;
 * every other BSON type falls back to its canonical EJSON text verbatim — round-trippable
 * through `resolveEjsonWrappers`, just not spelled as a constructor this app doesn't offer.
 */
function detectWrapper(value: Record<string, unknown>): ScalarRender | null {
  const keys = objectKeys(value);
  for (const spec of WRAPPER_TABLE) {
    if (!spec.match(keys, value)) continue;
    const rendered = spec.render(value);
    if (rendered) return rendered;
  }
  return null;
}

function renderScalar(value: string | number | boolean | null): ScalarRender {
  if (value === null) return { text: 'null', token: 'keyword', bsonType: 'json' };
  if (typeof value === 'boolean') {
    return { text: value ? 'true' : 'false', token: 'keyword', bsonType: 'json' };
  }
  if (typeof value === 'string') {
    return { text: JSON.stringify(value), token: 'string', bsonType: 'json' };
  }
  // Canonical EJSON never emits a bare JSON number for a BSON value, but tolerate one anyway
  // (e.g. hand-written test fixtures) rather than throwing.
  return { text: String(value), token: 'number', bsonType: 'json' };
}

function summaryFor(kind: 'object' | 'array', count: number): string {
  if (count === 0) return kind === 'object' ? '{}' : '[]';
  const noun =
    kind === 'object'
      ? t('studio.documents.row.fieldCount.status', { count })
      : t('studio.browse.count.status', { count });
  return kind === 'object' ? `{…} ${noun}` : `[…] ${noun}`;
}

function buildNode(value: unknown, key: string, path: string): DocNode {
  if (Array.isArray(value)) {
    const children = value.map((item, i) =>
      buildNode(item, String(i), path === '' ? String(i) : `${path}.${i}`),
    );
    return Object.freeze({
      key,
      path,
      kind: 'array',
      text: '',
      token: 'string',
      bsonType: 'json',
      children: Object.freeze(children),
      summary: summaryFor('array', children.length),
    });
  }
  if (isPlainObject(value)) {
    const wrapper = detectWrapper(value);
    if (wrapper) {
      return Object.freeze({
        key,
        path,
        kind: 'scalar',
        text: wrapper.text,
        token: wrapper.token,
        bsonType: wrapper.bsonType,
        children: Object.freeze([]),
        summary: '',
      });
    }
    const children = objectKeys(value).map((k) =>
      buildNode(value[k], k, path === '' ? k : `${path}.${k}`),
    );
    return Object.freeze({
      key,
      path,
      kind: 'object',
      text: '',
      token: 'string',
      bsonType: 'json',
      children: Object.freeze(children),
      summary: summaryFor('object', children.length),
    });
  }
  const scalar = renderScalar(value as string | number | boolean | null);
  return Object.freeze({
    key,
    path,
    kind: 'scalar',
    text: scalar.text,
    token: scalar.token,
    bsonType: scalar.bsonType,
    children: Object.freeze([]),
    summary: '',
  });
}

/** `null` for a body that is not a parseable JSON object — a truncated one (F3's 64 KB cut) or a
 *  non-object result. The caller falls back to showing the raw text (D22). */
export function parseDocument(body: string): DocNode | null {
  let parsed: unknown;
  try {
    parsed = JSON.parse(body);
  } catch {
    return null;
  }
  if (!isPlainObject(parsed)) return null;
  // A bare-object body could itself be a recognised wrapper (unlikely for a whole document, but
  // not impossible for a hand-crafted fixture) — buildNode's own object branch already checks
  // detectWrapper before falling back to the field-by-field walk, so this stays one code path.
  return buildNode(parsed, '', '');
}

/** The one-line label for a document's `_id`, from `DocumentPage.ids`' own EJSON text:
 *  '{"$oid":"507f…"}' -> { text: 'ObjectId("507f…")', bsonType: 'ObjectId' }. */
export function parseIdLabel(idEjson: string): { text: string; bsonType: BsonType } {
  let parsed: unknown;
  try {
    parsed = JSON.parse(idEjson);
  } catch {
    return { text: idEjson, bsonType: 'json' };
  }
  if (isPlainObject(parsed)) {
    const wrapper = detectWrapper(parsed);
    if (wrapper) return { text: wrapper.text, bsonType: wrapper.bsonType };
    return { text: JSON.stringify(parsed), bsonType: 'json' };
  }
  const scalar = renderScalar(parsed as string | number | boolean | null);
  return { text: scalar.text, bsonType: scalar.bsonType };
}

const INDENT = '  ';

function shellNodeText(node: DocNode, depth: number): string {
  const pad = INDENT.repeat(depth);
  const padIn = INDENT.repeat(depth + 1);
  if (node.kind === 'scalar') return node.text;
  if (node.kind === 'array') {
    if (node.children.length === 0) return '[]';
    const items = node.children.map((c) => `${padIn}${shellNodeText(c, depth + 1)}`);
    return `[\n${items.join(',\n')}\n${pad}]`;
  }
  if (node.children.length === 0) return '{}';
  const members = node.children.map(
    (c) => `${padIn}${JSON.stringify(c.key)}: ${shellNodeText(c, depth + 1)}`,
  );
  return `{\n${members.join(',\n')}\n${pad}}`;
}

/** The editable buffer's text: the document re-serialised as an indented Mongo shell literal —
 *  `ObjectId("…")`, `ISODate("…")`, `NumberLong("…")`, `NumberDecimal("…")`, `NumberInt(…)` for the
 *  six types literal.ts can construct, and the canonical extended-JSON object verbatim for every
 *  other type, so nothing is ever lossy. Falls back to the raw body unchanged when it does
 *  not parse (the raw-text case has nothing shell-literal to offer). */
export function toShellText(body: string): string {
  const root = parseDocument(body);
  if (!root) return body;
  return shellNodeText(root, 0);
}

// Canonical -> Relaxed Extended JSON v2.
// Reuses this file's own wrapper-shape helpers (isPlainObject/objectKeys/dateMillis) rather than
// a new BSON dependency: relaxed mode only ever changes number/date representation —
// $numberInt/$numberLong/a finite $numberDouble unwrap to a bare JSON number, and $date becomes
// an ISO-8601 string — every other wrapper ($oid, $numberDecimal, $binary, $timestamp,
// $regularExpression, $code, $ref/$id, $minKey/$maxKey) is unchanged in both modes per the spec,
// so it round-trips through the walk untouched.
function canonicalToRelaxed(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonicalToRelaxed);
  if (!isPlainObject(value)) return value;
  const keys = objectKeys(value);
  if (keys.length === 1 && keys[0] === '$numberInt' && typeof value.$numberInt === 'string') {
    const n = Number(value.$numberInt);
    if (Number.isFinite(n)) return n;
  }
  if (keys.length === 1 && keys[0] === '$numberLong' && typeof value.$numberLong === 'string') {
    const n = Number(value.$numberLong);
    // F9 (P108 Part 10): an Int64 above 2^53 is finite but not exactly representable as a JS
    // number — Number.isFinite(n) let e.g. 9007199254740993 silently unwrap to 9007199254740992.
    // isSafeInteger keeps anything beyond exact double precision wrapped, same as canonical.
    if (Number.isSafeInteger(n)) return n;
  }
  if (keys.length === 1 && keys[0] === '$numberDouble' && typeof value.$numberDouble === 'string') {
    const n = Number(value.$numberDouble);
    // NaN/Infinity have no bare-JSON-number spelling — stays wrapped, same as canonical.
    if (Number.isFinite(n)) return n;
    return value;
  }
  if (keys.length === 1 && keys[0] === '$date') {
    const millis = dateMillis(value.$date);
    if (millis !== null && millis >= 0 && millis <= MAX_RELAXED_DATE_MILLIS) {
      return { $date: new Date(millis).toISOString() };
    }
  }
  const out: Record<string, unknown> = {};
  for (const k of keys) out[k] = canonicalToRelaxed(value[k]);
  return out;
}

/** The document re-serialised as Relaxed Extended JSON — falls back to the raw body unchanged
 *  when it does not parse as JSON at all (same posture as toShellText above). */
export function toRelaxedText(body: string): string {
  let parsed: unknown;
  try {
    parsed = JSON.parse(body);
  } catch {
    return body;
  }
  return JSON.stringify(canonicalToRelaxed(parsed), null, 2);
}

// The normal/default copy action still produced JSON wrapped in Mongo/BSON type annotations,
// e.g. `{"$date": …}`/`{"$oid": …}`, not plain JSON:
// unlike canonicalToRelaxed above, this does not stop at "readable per the Extended JSON v2 spec"
// — Relaxed Extended JSON is *still* Extended JSON (autocomplete.spec.ts's own coverage locks in
// that `copy-relaxed-json` keeps `$oid` wrapped, "no relaxed variant for ObjectId" per the real
// spec; interaction.spec.ts locks in that `copy-as-json` (Canonical Extended
// JSON) round-trips through mongoimport/mongosh, which requires every wrapper to survive intact —
// so neither of those two, nor Shell mode, is the right thing to change). This resolves every
// wrapper detectWrapper recognises down to its plain-JSON equivalent instead: ObjectId/RegExp/Code
// become a plain string, Date becomes a plain ISO-8601 string, and every numeric wrapper
// (Int32/Int64/Decimal128/Double) becomes a plain JSON number — JS's usual floating-point
// precision then applies beyond Number.MAX_SAFE_INTEGER, the same accepted tradeoff any
// "export as plain JSON" tool makes; there is no lossless way to spell a 64-bit integer as a JSON
// number, which is exactly why this is a deliberately distinct, separately-offered format rather
// than a change to Canonical/Relaxed. Binary/Timestamp/DBRef/MinKey/MaxKey have no single scalar
// value to collapse into — their wrapper object survives, but every one of its keys has its
// leading `$` stripped, so the result never carries a `$`-prefixed key anywhere, unlike Canonical/
// Relaxed which both keep every wrapper verbatim.
function ejsonToPlain(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(ejsonToPlain);
  if (!isPlainObject(value)) return value;
  const keys = objectKeys(value);

  if (keys.length === 1 && keys[0] === '$oid' && typeof value.$oid === 'string') {
    return value.$oid;
  }
  if (keys.length === 1 && keys[0] === '$date') {
    const millis = dateMillis(value.$date);
    if (millis !== null) return new Date(millis).toISOString();
  }
  if (keys.length === 1 && keys[0] === '$numberInt' && typeof value.$numberInt === 'string') {
    return Number(value.$numberInt);
  }
  if (keys.length === 1 && keys[0] === '$numberLong' && typeof value.$numberLong === 'string') {
    return Number(value.$numberLong);
  }
  if (
    keys.length === 1 &&
    keys[0] === '$numberDecimal' &&
    typeof value.$numberDecimal === 'string'
  ) {
    return Number(value.$numberDecimal);
  }
  if (keys.length === 1 && keys[0] === '$numberDouble' && typeof value.$numberDouble === 'string') {
    const n = Number(value.$numberDouble);
    // Infinity/-Infinity/NaN have no bare-JSON-number spelling — the raw string is still plainer
    // than the `$numberDouble` wrapper it came out of.
    return Number.isFinite(n) ? n : value.$numberDouble;
  }
  if (
    keys.length === 1 &&
    keys[0] === '$regularExpression' &&
    isPlainObject(value.$regularExpression) &&
    typeof value.$regularExpression.pattern === 'string'
  ) {
    return value.$regularExpression.pattern;
  }
  if (
    (keys.length === 1 || keys.length === 2) &&
    keys.includes('$code') &&
    typeof value.$code === 'string' &&
    keys.every((k) => k === '$code' || k === '$scope')
  ) {
    return value.$code;
  }

  // Binary/Timestamp/DBRef/MinKey/MaxKey, or an ordinary object: recurse into every value, and
  // strip a leading `$` from every key along the way — the loop, not a length-1 special case,
  // since this same branch also has to walk an ordinary field-by-field document.
  const out: Record<string, unknown> = {};
  for (const k of keys) {
    const plainKey = k.startsWith('$') ? k.slice(1) : k;
    out[plainKey] = ejsonToPlain(value[k]);
  }
  return out;
}

/** The document re-serialised as genuinely plain JSON — no `$`-prefixed BSON wrapper anywhere,
 *  every recognised type resolved to its plain-JSON equivalent. Falls back to the raw body
 *  unchanged when it does not parse as JSON at all (same posture as toShellText/toRelaxedText). */
export function toPlainJson(body: string): string {
  let parsed: unknown;
  try {
    parsed = JSON.parse(body);
  } catch {
    return body;
  }
  return JSON.stringify(ejsonToPlain(parsed), null, 2);
}

// ---------------------------------------------------------------------------------------------
// Beautify/Minify for the document editor's shell-literal buffer. `beautify.ts`'s own
// JSON scanner can't reindent this text — a shell constructor call (`ObjectId("…")`) isn't valid
// JSON — so this is a small sibling scanner: the same lossless-raw-slice discipline, plus one more
// literal kind, a call expression, captured whole and never interpreted. Beautify/Minify reindent
// the buffer; they never re-serialise it or change a type.
// ---------------------------------------------------------------------------------------------

class ShellScanError extends Error {
  constructor(readonly offset: number) {
    super(`invalid document text at offset ${offset}`);
  }
}

function parseShellString(c: Cursor): string {
  const start = c.i;
  const quote = c.text[c.i];
  if (quote !== '"' && quote !== "'") throw new ShellScanError(c.i);
  c.i++;
  while (c.i < c.text.length && c.text[c.i] !== quote) {
    if (c.text[c.i] === '\\') c.i++;
    c.i++;
  }
  if (c.i >= c.text.length) throw new ShellScanError(c.i);
  c.i++;
  return c.text.slice(start, c.i);
}

function parseShellNumber(c: Cursor): string {
  const start = c.i;
  if (c.text[c.i] === '-') c.i++;
  while (/[0-9.eE+-]/.test(c.text[c.i] ?? '')) c.i++;
  if (c.i === start) throw new ShellScanError(c.i);
  return c.text.slice(start, c.i);
}

function parseShellIdentWord(c: Cursor): string {
  const start = c.i;
  while (/[A-Za-z0-9_$]/.test(c.text[c.i] ?? '')) c.i++;
  return c.text.slice(start, c.i);
}

// A constructor call's argument is opaque to this scanner — it only has to find the matching
// close paren (tracking nested parens and quoted strings), never interpret what's inside.
function parseShellCall(c: Cursor, identStart: number): string {
  let depth = 0;
  let quote: string | null = null;
  while (c.i < c.text.length) {
    const ch = c.text[c.i];
    if (quote) {
      if (ch === '\\') {
        c.i += 2;
        continue;
      }
      if (ch === quote) quote = null;
      c.i++;
      continue;
    }
    if (ch === '"' || ch === "'") {
      quote = ch;
      c.i++;
      continue;
    }
    if (ch === '(') depth++;
    if (ch === ')') {
      depth--;
      c.i++;
      if (depth === 0) return c.text.slice(identStart, c.i);
      continue;
    }
    c.i++;
  }
  throw new ShellScanError(c.i);
}

function parseShellValue(c: Cursor): RawNode {
  skipWs(c);
  const ch = c.text[c.i];
  if (ch === '{') return parseShellObject(c);
  if (ch === '[') return parseShellArray(c);
  if (ch === '"' || ch === "'") return { kind: 'literal', raw: parseShellString(c) };
  if (ch === '-' || (ch >= '0' && ch <= '9')) return { kind: 'literal', raw: parseShellNumber(c) };
  if (ch !== undefined && /[A-Za-z_$]/.test(ch)) {
    const identStart = c.i;
    const word = parseShellIdentWord(c);
    skipWs(c);
    if (c.text[c.i] === '(') return { kind: 'literal', raw: parseShellCall(c, identStart) };
    if (word === 'true' || word === 'false' || word === 'null' || word === 'undefined') {
      return { kind: 'literal', raw: word };
    }
    throw new ShellScanError(identStart);
  }
  throw new ShellScanError(c.i);
}

function parseShellKey(c: Cursor): string {
  skipWs(c);
  const ch = c.text[c.i];
  if (ch === '"' || ch === "'") return parseShellString(c);
  if (ch !== undefined && /[A-Za-z_$]/.test(ch)) return parseShellIdentWord(c);
  throw new ShellScanError(c.i);
}

function parseShellObject(c: Cursor): RawNode {
  return parseContainer(c, {
    parseKey: parseShellKey,
    parseValue: parseShellValue,
    skipWs,
    allowTrailingComma: true,
    error: (offset) => new ShellScanError(offset),
  });
}

function parseShellArray(c: Cursor): RawNode {
  return parseContainer(c, {
    parseValue: parseShellValue,
    skipWs,
    allowTrailingComma: true,
    error: (offset) => new ShellScanError(offset),
  });
}

// P42 D12: exported so views/console/lint.ts can validate a Mongo statement's argument against
// this app's own shell-literal grammar (unquoted keys, single quotes, ObjectId(…)/ISODate(…)
// calls) instead of JSON.parse, which would reject valid input this console actually accepts.
export function tryParseShellText(text: string): ParseResult {
  return tryParse(text, parseShellValue, skipWs, ShellScanError);
}

// An unquoted/single-quoted raw key is normalized through JSON.stringify — beautify/minify always
// emit a double-quoted key, even when the source used a bare identifier or single quotes.
const shellKeyText = (raw: string): string => JSON.stringify(raw.replace(/^['"]|['"]$/g, ''));

/**
 * Beautify/Minify for the document editor's edit buffer — reindents Mongo shell literal text
 * (JSON plus shell constructor calls) without re-serialising it or touching any value, including
 * a constructor call's own argument (opaque to this scanner). A buffer that fails to parse —
 * a hand edit gone wrong — reports the same shape `beautify.ts`'s JSON formatter does.
 */
export function beautifyShellText(text: string, mode: BeautifyMode): BeautifyResult {
  return beautifyWith(text, mode, tryParseShellText, 'document', shellKeyText);
}
