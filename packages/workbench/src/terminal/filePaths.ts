// Finds file path candidates in terminal text. Pure, no xterm import. Whether a candidate names an
// openable file is the backend's call; this only parses the `path[:line[:col]]` grammar.
//
// Not supported: paths with spaces, `path(line,col)`, `#L12` anchors, `a/`/`b/` diff prefixes,
// `file://` URLs, Windows paths.

export interface FilePathMatch {
  /** Index of the first char of the match in the text. */
  start: number;
  /** Exclusive; covers the `:line:col` suffix when present. */
  end: number;
  path: string;
  line: number | null;
  col: number | null;
}

export const MAX_MATCHES_PER_LINE = 20;
const MAX_PATH_LEN = 1024;
const MAX_POSITION_DIGITS = 7;

const TOKEN_BREAK = /[\s'"`()<>[\]{},;|]/;
const TRAILING_PUNCT = /[.,:;!?]+$/;
const POSITION_SUFFIX = /^(.*?):(\d+)(?::(\d+))?$/;
const EXTENSION = /\.[A-Za-z0-9]{1,10}$/;

function position(digits: string): number | null {
  if (digits.length > MAX_POSITION_DIGITS) return null;
  const n = Number(digits);
  return n > 0 ? n : null;
}

function qualifies(path: string): boolean {
  if (path === '' || path.length > MAX_PATH_LEN) return false;
  if (/^\.+$/.test(path) || path.startsWith('-') || path.includes('://')) return false;
  if (path.includes('/')) return true;
  return EXTENSION.test(path);
}

function parseToken(raw: string, start: number): FilePathMatch | null {
  const token = raw.replace(TRAILING_PUNCT, '');
  let path = token;
  let line: number | null = null;
  let col: number | null = null;
  let end = start + token.length;
  const m = POSITION_SUFFIX.exec(token);
  if (m) {
    path = m[1] ?? '';
    const l = position(m[2] ?? '');
    const c = m[3] === undefined ? null : position(m[3]);
    if (l !== null && (m[3] === undefined || c !== null)) {
      line = l;
      col = c;
    } else {
      end = start + path.length;
    }
  }
  if (!qualifies(path)) return null;
  return { start, end, path, line, col };
}

export function findFilePaths(text: string): FilePathMatch[] {
  const out: FilePathMatch[] = [];
  let spanStart = 0;
  let i = 0;
  while (i < text.length && out.length < MAX_MATCHES_PER_LINE) {
    const ch = text.charAt(i);
    if (/\s/.test(ch)) {
      spanStart = ++i;
      continue;
    }
    if (TOKEN_BREAK.test(ch)) {
      i++;
      continue;
    }
    let j = i;
    while (j < text.length && !TOKEN_BREAK.test(text.charAt(j))) j++;
    // A token cut out of a web link (`https://h/a,b/c.go` splits at the comma) is not a path.
    const inWebLink = text.slice(spanStart, i).includes('://');
    if (!inWebLink) {
      const match = parseToken(text.slice(i, j), i);
      if (match) out.push(match);
    }
    i = j;
  }
  return out;
}

export interface CellRow {
  /** Per cell, the string length it contributed to the row's text; 0 for the continuation cell of
   *  a wide char. An empty cell contributes 1 (`translateToString` renders it as a space). */
  chars: number[];
  /** Per cell, its display width (0, 1 or 2). */
  widths: number[];
}

export interface CellRange {
  startRow: number;
  startCol: number;
  endRow: number;
  /** Inclusive; covers the second cell of a wide char. */
  endCol: number;
}

/** Maps `[start, end)` of the rows' concatenated text onto 0-based cells. Null when out of range. */
export function toCellRange(rows: CellRow[], start: number, end: number): CellRange | null {
  if (end <= start) return null;
  let offset = 0;
  let startRow = -1;
  let startCol = -1;
  for (let r = 0; r < rows.length; r++) {
    const { chars, widths } = rows[r] as CellRow;
    for (let x = 0; x < chars.length; x++) {
      const len = chars[x] ?? 0;
      if (len === 0) continue;
      if (startRow < 0 && offset + len > start) {
        startRow = r;
        startCol = x;
      }
      offset += len;
      if (startRow >= 0 && offset >= end) {
        return { startRow, startCol, endRow: r, endCol: x + Math.max(widths[x] ?? 1, 1) - 1 };
      }
    }
  }
  return null;
}
