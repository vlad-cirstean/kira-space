import type { MonacoModule } from './monaco';

// `AutocompleteField.vue`'s overlay is paint-only — a full editor per filter field is the wrong
// shape. `monaco.editor.colorize()` already does the
// language -> CSS class mapping this needs (it is what backs the app-wide "kira-editor" theme's
// `mtk*` colours), but its HTML is one flat run per token with no offsets — this module re-derives
// offsets by walking that HTML once, then merges in `{{variable}}`/range-highlight boundaries as a
// second, independent pass so the two never fight over who "owns" a span.

/** A run of text carrying zero or more CSS classes, positioned by document offset. Output of
 *  `mergeHighlightRanges` — never overlapping, never gapped inside `[0, textLength)`. */
export interface ClassRun {
  from: number;
  to: number;
  classes: readonly string[];
}

interface BaseRun {
  from: number;
  to: number;
  classes: readonly string[];
}

interface HighlightRange {
  from: number;
  to: number;
  class: string;
}

/** Pure boundary merge over two sorted-by-construction interval lists: `base` (syntax-colour runs,
 *  expected to tile `[0, textLength)` with no gaps) and `ranges` (`{{variable}}`/find-match
 *  highlights, arbitrary and possibly overlapping each other). Every output run's classes are its
 *  covering base run's classes plus every range's class that covers it — a run with no classes at
 *  all (plain text, no highlight) is dropped, since the caller renders it as bare text.
 *
 *  Degenerate inputs are the point of this function's own unit test: a zero-width or
 *  out-of-range `range` (`from >= to`, `from < 0`, `to > textLength`) is clamped or dropped rather
 *  than thrown — the same "one keystroke of missing colour, never a crash" rule
 *  `variableHighlight.ts`'s own `buildDecorations` already keeps for CodeMirror. */
export function mergeHighlightRanges(
  textLength: number,
  base: readonly BaseRun[],
  ranges: readonly HighlightRange[],
): ClassRun[] {
  const validRanges = ranges
    .map((r) => ({ ...r, from: Math.max(0, r.from), to: Math.min(textLength, r.to) }))
    .filter((r) => r.from < r.to);

  const cuts = new Set<number>([0, textLength]);
  for (const b of base) {
    cuts.add(Math.max(0, Math.min(textLength, b.from)));
    cuts.add(Math.max(0, Math.min(textLength, b.to)));
  }
  for (const r of validRanges) {
    cuts.add(r.from);
    cuts.add(r.to);
  }
  const points = [...cuts].sort((a, b) => a - b);

  const out: ClassRun[] = [];
  // `base` tiles `[0, textLength)` left to right with no gaps (this function's own doc comment;
  // `parseColorizedLine`'s only caller builds it that way), and `points` is sorted ascending, so a
  // single forward-only pointer finds each point's covering base run in amortized O(1) instead of
  // rescanning all of `base` per point (O(P*B) total before this — a real cost on a long,
  // heavily-tokenized line under `ranges` highlighting on every keystroke).
  let baseIdx = 0;
  for (let i = 0; i < points.length - 1; i++) {
    const from = points[i];
    const to = points[i + 1];
    if (from >= to) continue;
    while (baseIdx < base.length && base[baseIdx].to <= from) baseIdx++;
    const baseRun =
      baseIdx < base.length && base[baseIdx].from <= from && from < base[baseIdx].to
        ? base[baseIdx]
        : undefined;
    const classes = [...(baseRun?.classes ?? [])];
    for (const r of validRanges) {
      if (r.from <= from && from < r.to) classes.push(r.class);
    }
    if (classes.length === 0) continue;
    // Coalesce into the previous run when it ends exactly here and carries the same classes in the
    // same order — a `base` cut that falls inside a highlight (e.g. `colorize()`'s own chunking of
    // one token's text across two `mtkN` spans, unrelated to this function's own callers) would
    // otherwise split one semantic highlight into two adjacent DOM spans with identical classes.
    const prev = out[out.length - 1];
    if (prev && prev.to === from && sameClasses(prev.classes, classes)) {
      prev.to = to;
    } else {
      out.push({ from, to, classes });
    }
  }
  return out;
}

function sameClasses(a: readonly string[], b: readonly string[]): boolean {
  return a.length === b.length && a.every((c, i) => c === b[i]);
}

// `monaco.editor.colorize()`'s own HTML shape (verified against the pinned 0.56.0's
// `viewLineRenderer.js`): one line is `<span>` wrapping a flat run of
// `<span class="mtkN">…text…</span>` (entities `&lt;`/`&gt;`/`&amp;` only — colorize is called with
// `renderWhitespace` off, so no `mtkw`/`mtkz` width-span case ever appears here), terminated by
// `<br/>`. AutocompleteField's overlay is always single-line (`singleLine` was CodeMirrorHost's own
// contract for this component; there is exactly one line to parse.
//
// The same renderer also rewrites every plain
// U+0020 space into U+00A0 (NBSP) unconditionally, not just inside a 2+-space run — verified
// against a real `colorize()` call, a single space either side of `|` in `{{base_url | base64}}`
// came back as NBSP. Not an HTML entity (`&nbsp;` never appears in the string), so it isn't caught
// by an entity table — decoded back to a plain space explicitly, or every `parsedText === text`
// comparison downstream would falsely "mismatch" on any colorized text containing a space and lose
// every range highlight over it (`paintOverlayHtml`'s own shape-changed fallback).
const TOKEN_SPAN_RE = /<span class="([^"]+)">([\s\S]*?)<\/span>/g;

function decodeEntities(s: string): string {
  return s
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&amp;/g, '&')
    .replace(/\u00a0/g, ' '); // NBSP -> space, see the comment above `TOKEN_SPAN_RE`.
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

/** Parses one line of `colorize()`'s HTML into offset-positioned base runs. Exported for the unit
 *  test; not meant to be called outside this module in production code. */
export function parseColorizedLine(html: string): { text: string; runs: BaseRun[] } {
  let text = '';
  const runs: BaseRun[] = [];
  TOKEN_SPAN_RE.lastIndex = 0;
  for (let match = TOKEN_SPAN_RE.exec(html); match !== null; match = TOKEN_SPAN_RE.exec(html)) {
    const className = match[1];
    const decoded = decodeEntities(match[2]);
    const from = text.length;
    text += decoded;
    runs.push({ from, to: text.length, classes: [className] });
  }
  return { text, runs };
}

/** Renders `text` as syntax-highlighted, `{{variable}}`/find-match-highlighted HTML — the whole of
 *  `AutocompleteField.vue`'s overlay in one call. `colorize()` is `async` (it correctly awaits a
 *  Monarch grammar's own lazy load, `monacoEntry.ts`'s `registerTokensProviderFactory`); the first
 *  paint before it resolves is plain, unstyled text, same graceful degradation `MonacoHost.vue`'s
 *  own pending state uses for the same reason. */
export async function paintOverlayHtml(
  mod: MonacoModule,
  text: string,
  languageId: string,
  ranges: readonly HighlightRange[],
): Promise<string> {
  if (text === '') return '';
  const colorized = await mod.editor.colorize(text, languageId, { tabSize: 2 });
  const { text: parsedText, runs } = parseColorizedLine(colorized);
  // `plaintext` (the URL field's own language, and every field with no grammar) has no tokenizer,
  // so colorize() returns the text with no `mtkN` spans at all — `runs` comes back empty and
  // `parsedText` mismatches (`''` vs. the real text). That is expected, not a shape change to
  // degrade from: fall back to one classless base run spanning the whole text so `ranges`
  // (`{{variable}}` colouring, find-match highlights) still paints over plain text. Only an
  // actual mismatch with a non-trivial `runs` list (colorize() produced spans, but the
  // concatenated text does not match) means the HTML shape changed under us — that one degrades to
  // plain escaped text rather than risk mis-painting.
  if (parsedText !== text && runs.length === 0) return paintRangesHtml(text, ranges);
  if (parsedText !== text) return escapeHtml(text);
  return runsToHtml(text, mergeHighlightRanges(text.length, runs, ranges));
}

function runsToHtml(text: string, merged: readonly ClassRun[]): string {
  let html = '';
  let cursor = 0;
  for (const run of merged) {
    if (run.from > cursor) html += escapeHtml(text.slice(cursor, run.from));
    html += `<span class="${run.classes.join(' ')}">${escapeHtml(text.slice(run.from, run.to))}</span>`;
    cursor = run.to;
  }
  if (cursor < text.length) html += escapeHtml(text.slice(cursor));
  return html;
}

/** One classless base run plus `ranges`: the ranges-only overlay needs no tokenizer. */
export function paintRangesHtml(text: string, ranges: readonly HighlightRange[]): string {
  if (text === '') return '';
  return runsToHtml(
    text,
    mergeHighlightRanges(text.length, [{ from: 0, to: text.length, classes: [] }], ranges),
  );
}

/** `document.caretPositionFromPoint`/`caretRangeFromPoint` (WebKit ships the latter only, as of
 *  this pin) walked back to a document character offset — DOM-native, exact, and font-agnostic,
 *  the two properties CodeMirror's coordinate hit-testing was chosen *for* originally.
 *  `null` when the point falls outside the overlay entirely, matching
 *  `MonacoHost.posAtCoords`'s own contract. */
export function overlayOffsetAtPoint(overlayEl: HTMLElement, x: number, y: number): number | null {
  const doc = overlayEl.ownerDocument;
  let node: Node | null = null;
  let nodeOffset = 0;
  if (typeof doc.caretPositionFromPoint === 'function') {
    const pos = doc.caretPositionFromPoint(x, y);
    if (!pos) return null;
    node = pos.offsetNode;
    nodeOffset = pos.offset;
  } else if (typeof doc.caretRangeFromPoint === 'function') {
    const range = doc.caretRangeFromPoint(x, y);
    if (!range) return null;
    node = range.startContainer;
    nodeOffset = range.startOffset;
  } else {
    return null;
  }
  if (!node || !overlayEl.contains(node)) return null;
  const walker = doc.createTreeWalker(overlayEl, NodeFilter.SHOW_TEXT);
  let total = 0;
  for (let current = walker.nextNode(); current; current = walker.nextNode()) {
    if (current === node) return total + nodeOffset;
    total += current.textContent?.length ?? 0;
  }
  return null;
}
