import { beautifyJson } from '../../beautify';
import {
  beautifyShellText,
  toPlainJson,
  toRelaxedText,
  toShellText,
} from '../shared/document/ejson';

export type CopyAllFormat = 'plain' | 'shell' | 'canonical' | 'relaxed';

// D6: the row's body -- already canonical extended JSON (a Mongo document) or built fresh (a
// Redis key/value pair) -- re-indented through beautify.ts's JSON scanner, falling back to the
// raw text if it does not scan (a truncated body, say).
export function prettyJson(text: string): string {
  const r = beautifyJson(text, 'indented');
  return r.ok ? r.text : text;
}

function indented(text: string): string {
  return text
    .split('\n')
    .map((line) => `  ${line}`)
    .join('\n');
}

// D6's own "copy all" shape: `[` + every row's (already-pretty) text, comma-joined + `]` -- a
// plain textual assembly, not a re-parse-and-restringify, so a row whose own text doesn't happen
// to scan still lands in the array unindented rather than dropping it.
export function jsonArrayOf(items: readonly string[]): string {
  return `[\n${items.map((item) => indented(item)).join(',\n')}\n]`;
}

// `toShellText` always
// pretty-prints -- its one caller before this was the document editor's own buffer, where an
// indented literal is exactly what's wanted. "Copy all", one document per line, needs the
// opposite: `beautifyShellText`'s existing 'compact' mode re-renders shell text without its own
// newlines, so composing the two here (rather than teaching toShellText a second mode it has
// exactly one non-editor caller for) gives each document its own single line.
function compactShellText(body: string): string {
  const indentedText = toShellText(body);
  const compacted = beautifyShellText(indentedText, 'compact');
  return compacted.ok ? compacted.text : indentedText;
}

/** Every displayed document in one copy format; runs inline for small pages, on the parse worker for large ones. */
export function copyAllDocuments(format: CopyAllFormat, bodies: readonly string[]): string {
  switch (format) {
    case 'plain':
      return jsonArrayOf(bodies.map(toPlainJson));
    case 'shell':
      return bodies.map(compactShellText).join('\n');
    case 'canonical':
      return jsonArrayOf(bodies.map(prettyJson));
    case 'relaxed':
      return jsonArrayOf(bodies.map(toRelaxedText));
  }
}
