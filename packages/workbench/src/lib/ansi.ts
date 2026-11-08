import Anser from 'anser';

export interface AnsiSegment {
  text: string;
  color?: string;
  background?: string;
  bold: boolean;
  faint: boolean;
}

const rgb = (v: string | null | undefined): string | undefined => (v ? `rgb(${v})` : undefined);

/** A log's chunks as plain text, one line each: ANSI escapes and the trailing newline dropped. */
export function logPlainText(chunks: readonly { text: string }[]): string {
  return chunks.map((c) => Anser.ansiToText(c.text).replace(/\n$/, '')).join('\n');
}

/** One log line's ANSI escapes as styled text segments. */
export function parseAnsi(line: string): AnsiSegment[] {
  if (!line.includes('\u001b')) return [{ text: line, bold: false, faint: false }];
  return Anser.ansiToJson(line, { json: true, remove_empty: true }).map((e) => ({
    text: e.content,
    color: rgb(e.fg),
    background: rgb(e.bg),
    bold: e.decorations.includes('bold'),
    faint: e.decorations.includes('dim'),
  }));
}
