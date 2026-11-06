import { z } from 'zod';

// zod schemas mirroring internal/storage/model/maskrule.go, and a pure-function port
// of internal/mask/mask.go — "function for function", pinned against the same shared fixture set
// (apps/kira-space/tests/fixtures/studio/mask/*.{input,expected}.json) both apps/kira-space/internal/
// mask/parity_test.go and tests/unit/mask-parity.spec.ts read, the explain-plan pattern.
//
// Six kinds, not seven: an `id` kind (identifier columns, tag-only) was dropped before
// implementation — internal ids are not PII, so no rule should apply to them at all; a column
// meant to stay untouched carries no rule, rather than one whose kind is a no-op.
export const maskKindSchema = /*#__PURE__*/ z.enum([
  'name',
  'email',
  'text',
  'number',
  'date',
  'redact',
]);
export type MaskKind = z.infer<typeof maskKindSchema>;

// maskRuleFieldsSchema mirrors model.MaskRuleFields — what the Upsert bridge call accepts.
export const maskRuleFieldsSchema = /*#__PURE__*/ z.object({
  tableName: z.string(),
  columnName: z.string(),
  kind: maskKindSchema,
  keepHint: z.boolean(),
  correlate: z.boolean(),
});
export type MaskRuleFields = z.infer<typeof maskRuleFieldsSchema>;

// maskRuleSchema mirrors model.MaskRule — one connection_mask_rules row.
export const maskRuleSchema = /*#__PURE__*/ maskRuleFieldsSchema.extend({
  id: z.string(),
  connectionId: z.string(),
  createdAt: z.string(),
  updatedAt: z.string(),
});
export type MaskRule = z.infer<typeof maskRuleSchema>;

// --- the masking algorithm itself (internal/mask/mask.go's own port) ---

// MaskingRule is the bare {kind, keepHint, correlate} triple internal/mask.Rule carries — never a
// table/column name (matching happens one layer up, before masking is ever applied), and
// deliberately not the same type as MaskRuleFields above: a stored row and "what Apply needs" are
// different shapes in Go too (model.MaskRuleFields vs mask.Rule).
export interface MaskingRule {
  kind: MaskKind;
  keepHint: boolean;
  correlate: boolean;
}

// redactLiteral/bullet mirror mask.go's own unexported constants exactly.
const REDACT_LITERAL = '[redacted]';
const BULLET = '•'; // U+2022 — not `*`, which reads as SQL/shell syntax.

// graphemeSegmenter is UAX #29 grapheme-cluster segmentation via Intl.Segmenter, the same standard
// github.com/rivo/uniseg implements on the Go side — length hints count graphemes, not UTF-16 code
// units, so a masked CJK or emoji-bearing value does not lie about its size. Hoisted to module
// scope: constructing one per call — as every cell in a masked grid column did,
// once per render — is real, avoidable setup cost Intl.Segmenter's own construction carries; the
// segmenter itself holds no per-string state, so one shared instance is safe to reuse.
const graphemeSegmenter = new Intl.Segmenter(undefined, { granularity: 'grapheme' });

// graphemeCount returns s's grapheme-cluster count without materializing every cluster into an
// array — the shape maskText and maskEmail's domain-hint-off case need (a count alone).
function graphemeCount(s: string): number {
  if (s === '') return 0;
  let n = 0;
  for (const _ of graphemeSegmenter.segment(s)) n++;
  return n;
}

// firstGraphemeAndCount returns s's first grapheme cluster (empty when s is empty) alongside its
// total count in one segmenter pass — maskWord's own need, without allocating an array of every
// cluster just to keep two of them.
function firstGraphemeAndCount(s: string): { first: string; count: number } {
  if (s === '') return { first: '', count: 0 };
  let first = '';
  let count = 0;
  for (const seg of graphemeSegmenter.segment(s)) {
    if (count === 0) first = seg.segment;
    count++;
  }
  return { first, count };
}

// maskWord masks one word: first grapheme + bullet run when keepHint, an all-bullet run of the
// same grapheme length otherwise. A single-grapheme word (n===1) always gets the all-bullet form
// regardless of keepHint — "keep the first grapheme, destroy the rest" has zero characters left to
// destroy when the whole word is one grapheme, so honouring keepHint there would echo the word
// back whole. The length hint survives intact either way (a single bullet still says "one
// grapheme"); only the content does not.
function maskWord(word: string, keepHint: boolean): string {
  const { first, count: n } = firstGraphemeAndCount(word);
  if (n === 0) return '';
  if (!keepHint || n === 1) return BULLET.repeat(n);
  return first + BULLET.repeat(n - 1);
}

// GO_SPACE is Go's unicode.IsSpace set, spelled out: JS `\s` matches U+FEFF (Go does not) and
// misses U+0085 (Go matches), so a name or number containing either would otherwise split or trim
// differently here than in the Go render path.
const GO_SPACE =
  '[\\t\\n\\v\\f\\r \\u0085\\u00A0\\u1680\\u2000-\\u200A\\u2028\\u2029\\u202F\\u205F\\u3000]';
const NAME_WORD_SPLIT_RE = new RegExp(`${GO_SPACE}+`);
const GO_TRIM_RE = new RegExp(`^${GO_SPACE}+|${GO_SPACE}+$`, 'g');

// maskName keeps each whitespace-separated word's first grapheme (when keepHint) plus its own
// grapheme length, destroying every other character. A value with no words at all (empty after
// trimming whitespace — not the empty-string case apply already handles) falls through to
// REDACT_LITERAL rather than echoing "".
function maskName(value: string, keepHint: boolean): string {
  const words = value.split(NAME_WORD_SPLIT_RE).filter((w) => w.length > 0);
  if (words.length === 0) return REDACT_LITERAL;
  return words.map((w) => maskWord(w, keepHint)).join(' ');
}

// maskEmail keeps the domain (in full, when keepHint) and the local part's first grapheme +
// length. A value with no `@` is not an email — it falls through to maskName's own shape, never
// emitting more than that would.
function maskEmail(value: string, keepHint: boolean): string {
  const i = value.lastIndexOf('@');
  if (i < 0) return maskName(value, keepHint);
  const local = value.slice(0, i);
  const domain = value.slice(i + 1);
  if (local === '') {
    // M7 finding: an empty local part reassembles as `${''}@${domain}` — the input itself,
    // verbatim, whenever keepHint is true. No path may echo the original text; fall through to the
    // universal redaction instead.
    return REDACT_LITERAL;
  }
  const localMasked = maskWord(local, keepHint);
  const domainMasked = keepHint ? domain : BULLET.repeat(graphemeCount(domain));
  return `${localMasked}@${domainMasked}`;
}

// TEXT_BUCKET_EDGES are the fixed, data-independent boundaries §2.3 names: 0 exactly, then
// doubling from 8. Never derived from the column's own distribution.
const TEXT_BUCKET_EDGES = [1, 8, 16, 32, 64, 128, 256, 512];

function lengthBucket(n: number): string {
  if (n <= 0) return '0';
  for (let i = 1; i < TEXT_BUCKET_EDGES.length; i++) {
    if (n < TEXT_BUCKET_EDGES[i]) {
      return `${TEXT_BUCKET_EDGES[i - 1]}-${TEXT_BUCKET_EDGES[i]}`;
    }
  }
  const last = TEXT_BUCKET_EDGES[TEXT_BUCKET_EDGES.length - 1];
  return `${last}+`;
}

// MASK_TEXT_GRAPHEME_CAP is TEXT_BUCKET_EDGES' own top edge. lengthBucket only ever needs to tell
// "n is below this edge" from "n is at or beyond it" — every count at or beyond it collapses to
// the same "512+" bucket — so boundedGraphemeCount can break out of the segmenter loop the instant
// it proves that, rather than segmenting the rest of a very long value for a distinction the
// bucket label never surfaces.
const MASK_TEXT_GRAPHEME_CAP = 512;

// boundedGraphemeCount mirrors graphemeCount but stops iterating once the count reaches limit. For
// any string with fewer than limit graphemes this returns the exact count (identical to
// graphemeCount); for a longer one it returns limit — sufficient for lengthBucket, which treats
// every n >= limit the same.
function boundedGraphemeCount(s: string, limit: number): number {
  if (s === '') return 0;
  let n = 0;
  for (const _ of graphemeSegmenter.segment(s)) {
    n++;
    if (n >= limit) break;
  }
  return n;
}

// maskText keeps nothing but a bucketed length — no first character, no exact length.
function maskText(value: string): string {
  const n = boundedGraphemeCount(value, MASK_TEXT_GRAPHEME_CAP);
  return `${BULLET}${BULLET}${BULLET} (text, ${lengthBucket(n)} chars)`;
}

// NUMERIC_RE is the canonical accept/reject gate for maskNumber, on both sides (mask.go's own
// numericAcceptRE mirrors this exactly, checked before its ParseFloat call): plain-decimal and
// scientific-notation shapes only. Deliberately narrower than strconv.ParseFloat's own Go-literal
// grammar (which additionally accepts `_` digit separators and hex floats like "0x1p4") — a real
// DB driver never emits either as a numeric column's text representation, so this stays the
// stricter, shared shape rather than widening Go to match a grammar TS's permissive `Number()`
// already had to be walled off from (hex literals, "", whitespace coercing to 0).
const NUMERIC_RE = /^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$/;

// PLAIN_DECIMAL_RE matches an unsigned plain decimal literal — digits, optionally one `.` and more
// digits. Anything else (scientific notation, garbage) falls back to the float-based exponent,
// which is not guaranteed exact at a decade boundary.
const PLAIN_DECIMAL_RE = /^[0-9]+(\.[0-9]+)?$/;

// decadeExponent returns k such that 10^k <= absVal < 10^(k+1), computed by counting digits in
// absStr whenever it is a plain decimal literal — Math.log10 of an exact power of ten can land a
// hair under the true integer, misfiling a boundary value into the wrong decade.
function decadeExponent(absStr: string, absVal: number): number {
  if (PLAIN_DECIMAL_RE.test(absStr)) {
    let intPart = absStr;
    let fracPart = '';
    let hasDot = false;
    const dotIdx = absStr.indexOf('.');
    if (dotIdx >= 0) {
      intPart = absStr.slice(0, dotIdx);
      fracPart = absStr.slice(dotIdx + 1);
      hasDot = true;
    }
    intPart = intPart.replace(/^0+/, '');
    if (intPart !== '') return intPart.length - 1;
    if (hasDot) {
      let lead = 0;
      while (lead < fracPart.length && fracPart[lead] === '0') lead++;
      return -(lead + 1);
    }
  }
  return Math.floor(Math.log10(absVal));
}

// pow10String renders 10^k exactly, as a decimal string — never via floating-point Math.pow, which
// cannot represent most negative powers of ten exactly.
function pow10String(k: number): string {
  if (k >= 0) return `1${'0'.repeat(k)}`;
  return `0.${'0'.repeat(-k - 1)}1`;
}

// decadeRange formats value's own fixed-power-of-ten bucket. ok is false when value does not parse
// as a real number at all.
function decadeRange(value: string): { range: string; ok: boolean } {
  const trimmed = value.replace(GO_TRIM_RE, '');
  if (!NUMERIC_RE.test(trimmed)) return { range: '', ok: false };
  const v = Number(trimmed);
  if (!Number.isFinite(v)) return { range: '', ok: false };
  if (v === 0) return { range: '0', ok: true };
  const neg = v < 0;
  const absVal = Math.abs(v);
  const absStr = neg ? trimmed.replace(/^-/, '') : trimmed.replace(/^\+/, '');
  const k = decadeExponent(absStr, absVal);
  const lower = pow10String(k);
  const upper = pow10String(k + 1);
  return neg
    ? { range: `(-${upper}--${lower}]`, ok: true }
    : { range: `[${lower}-${upper})`, ok: true };
}

// maskNumber keeps the sign and a fixed decade range, never a digit. A value that does not parse
// as a number falls through to REDACT_LITERAL — never an echo, and never a tag either (apply()
// below enforces "a number never carries a correlation tag" regardless of this function's output).
function maskNumber(value: string): string {
  const { range, ok } = decadeRange(value);
  return ok ? range : REDACT_LITERAL;
}

// DATE_LEADING_YEAR_RE recognises a leading ISO-8601 YYYY-MM-DD. Anything else falls through to
// maskText rather than guessing at a shape to preserve.
const DATE_LEADING_YEAR_RE = /^\d{4}-\d{2}-\d{2}/;

function destroyDigits(s: string): string {
  let out = '';
  for (const ch of s) {
    out += /\p{Nd}/u.test(ch) ? BULLET : ch;
  }
  return out;
}

// maskDate keeps the year (when keepHint) and destroys every digit after it — month, day, and
// every time component — while preserving every separator, so the output still shows the value's
// own granularity.
function maskDate(value: string, keepHint: boolean): string {
  if (!DATE_LEADING_YEAR_RE.test(value)) return maskText(value);
  const year = keepHint ? value.slice(0, 4) : BULLET.repeat(4);
  return year + destroyDigits(value.slice(4));
}

// applyVisible dispatches by kind and returns the redaction alone, before any tag is appended.
// Every arm is total — an unrecognised kind falls to the same REDACT_LITERAL a real `redact` rule
// produces (fail closed, everywhere, with no exceptions).
export function applyVisible(rule: MaskingRule, value: string): string {
  switch (rule.kind) {
    case 'name':
      return maskName(value, rule.keepHint);
    case 'email':
      return maskEmail(value, rule.keepHint);
    case 'text':
      return maskText(value);
    case 'number':
      return maskNumber(value);
    case 'date':
      return maskDate(value, rule.keepHint);
    case 'redact':
      return REDACT_LITERAL;
    default:
      return REDACT_LITERAL;
  }
}

// CROCKFORD_ALPHABET excludes I, L, O, U so a tag never reads as a word and never confuses 0/O
// when a human retypes it — the same alphabet internal/mask's crockfordEncoding uses. Exported:
// celleditor/generate.ts's ULID generator used to carry an identical copy for its own
// timestamp-half encoder.
export const CROCKFORD_ALPHABET = '0123456789ABCDEFGHJKMNPQRSTVWXYZ';

// crockfordBase32 mirrors Go's encoding/base32 standard bit-packing (5 bits per character, MSB
// first across the byte stream, no padding) — verified byte-for-byte against
// internal/mask.tag's own output via the parity fixtures' fixed-key-tag-vector case. Exported:
// celleditor/generate.ts's own ULID random-half encoder was a duplicate of this exact
// bit-packing (the only difference was an unreachable trailing-pad branch there had already been
// stripped — see this function's own trailing `if (bitCount > 0)` below, which only fires for a
// non-multiple-of-5 bit length; the ULID caller always feeds it 80 bits, so it never fires there
// either, making the two byte-for-byte interchangeable for that input).
export function crockfordBase32(bytes: Uint8Array): string {
  let bitBuffer = 0;
  let bitCount = 0;
  let out = '';
  for (const byte of bytes) {
    bitBuffer = (bitBuffer << 8) | byte;
    bitCount += 8;
    while (bitCount >= 5) {
      out += CROCKFORD_ALPHABET[(bitBuffer >>> (bitCount - 5)) & 31];
      bitCount -= 5;
    }
  }
  if (bitCount > 0) {
    out += CROCKFORD_ALPHABET[(bitBuffer << (5 - bitCount)) & 31];
  }
  return out;
}

// importedHmacKeys caches each key's imported CryptoKey, keyed by the Uint8Array object itself:
// buildMaskTagCache (maskPreview.ts) calls tag() once per distinct value on a
// page — all sharing the same key — and importKey is as async and non-trivial as sign() itself, so
// without this every one of those calls re-imported an identical key from scratch. A WeakMap, not
// a plain Map: the cache entry disappears on its own once the key bytes it was built from do,
// nothing here needs to evict it by hand.
const importedHmacKeys = new WeakMap<Uint8Array, Promise<CryptoKey>>();

function importHmacKey(key: Uint8Array): Promise<CryptoKey> {
  let imported = importedHmacKeys.get(key);
  if (!imported) {
    imported = crypto.subtle.importKey(
      'raw',
      key.buffer as ArrayBuffer,
      { name: 'HMAC', hash: 'SHA-256' },
      false,
      ['sign'],
    );
    importedHmacKeys.set(key, imported);
  }
  return imported;
}

// tag computes "#" + crockfordBase32(HMAC-SHA256(key, value))[0:6] — exactly internal/mask's own
// formula. The HMAC message is value's raw UTF-8 bytes alone, nothing else. Async: Web Crypto's
// HMAC has no synchronous API, which is why the grid preview (maskPreview.ts) precomputes tags
// into a cache ahead of SlickGrid's synchronous cellFormatter rather than calling this per cell.
export async function tag(key: Uint8Array, value: string): Promise<string> {
  const cryptoKey = await importHmacKey(key);
  const signature = await crypto.subtle.sign('HMAC', cryptoKey, new TextEncoder().encode(value));
  const encoded = crockfordBase32(new Uint8Array(signature));
  return `#${encoded.slice(0, 6)}`;
}

// apply masks one non-NULL cell — mirrors internal/mask.Masker.Apply exactly, including the
// number-never-tags enforcement (defensive, on top of the storage-layer validation that already
// sets correlate=false for a `number` rule). key null or empty means no correlation key exists for
// this connection yet — Apply then never calls tag, degrading gracefully to the redaction alone
// (never a leak, only less information).
export async function apply(
  rule: MaskingRule,
  key: Uint8Array | null,
  value: string,
): Promise<string> {
  if (value === '') return '';
  const visible = applyVisible(rule, value);
  if (rule.kind === 'number' || !rule.correlate || key === null || key.length === 0) {
    return visible;
  }
  return visible + (await tag(key, value));
}

// maskNullable applies apply() to value's own content when value is non-null, and passes NULL
// through unchanged otherwise — masking a NULL would invent a value that is not there.
export async function maskNullable(
  rule: MaskingRule,
  key: Uint8Array | null,
  value: string | null,
): Promise<string | null> {
  if (value === null) return null;
  return apply(rule, key, value);
}

// KIND_STRICTNESS ranks the six kinds from most to least redacting (the conflict-folding
// order): redact > text > date > number > name > email. Lower rank wins a conflict.
//
// email and number previously sat the other way around (redact > text > date > email > name >
// number), backwards for two pairs relative to actual redaction strength. email vs
// name: `email` leaves the domain fully visible while `name` bullets far more of the value, so
// `email` must not outrank `name`. name vs number: `number` fully brackets a value (no digits at
// all) while `name` keeps a leading character and reveals the exact length, so `number` must
// outrank `name`. Swapping only email's and number's own rank values (name's is unchanged)
// satisfies both — kept identical to mask.go's own kindStrictness, including this reasoning.
const KIND_STRICTNESS: Record<MaskKind, number> = {
  redact: 0,
  text: 1,
  date: 2,
  number: 3,
  name: 4,
  email: 5,
};

// stricter returns whichever of a, b is the stricter rule: the lower-ranked kind wins, and the two
// flags fold independently toward their own stricter (more redacting) value — keepHint and
// correlate each false-wins — across any kind pair, not only when a and b share a kind (a stricter
// flag on the losing kind's own rule must not be silently discarded
// just because its kind lost). An ambiguity between two rules matching the same column name (under
// different table names) must never resolve to the more permissive option on any axis. Used by the
// grid preview to fold its own copy of a connection's rules by column name, the same way
// maskrules.Service.MaskSetFor folds them server-side for the MCP render path.
export function stricter(a: MaskingRule, b: MaskingRule): MaskingRule {
  const ra = KIND_STRICTNESS[a.kind] ?? KIND_STRICTNESS.redact;
  const rb = KIND_STRICTNESS[b.kind] ?? KIND_STRICTNESS.redact;
  const kind = rb < ra ? b.kind : a.kind;
  return {
    kind,
    keepHint: a.keepHint && b.keepHint,
    correlate: a.correlate && b.correlate,
  };
}

// foldRulesByColumn mirrors maskrules.Service.MaskSetFor's own fold (§4.2): every rule on the
// connection collapses to one per lowercased column name, ignoring table_name (matching is by
// column name alone, case-insensitively, across every rule on the connection).
export function foldRulesByColumn(rules: MaskRuleFields[]): Map<string, MaskingRule> {
  const folded = new Map<string, MaskingRule>();
  for (const rule of rules) {
    const key = rule.columnName.toLowerCase();
    const bare: MaskingRule = {
      kind: rule.kind,
      keepHint: rule.keepHint,
      correlate: rule.correlate,
    };
    const existing = folded.get(key);
    folded.set(key, existing ? stricter(existing, bare) : bare);
  }
  return folded;
}
