// Finds file path candidates in terminal text. Pure, no xterm import. Whether a candidate names an
// openable file is the backend's call; this only parses the path grammar: `path` plus an optional
// position (`:L`, `:L:C`, `:L-L2`, `:L:C-C2`, `#LL`, `#LLCC`, `#LL-LL2`, `(L)`, `(L,C)`, Python's
// `", line L`), quoted spans with spaces, `\ ` escapes and `file://` URLs.
//
// Not supported: Windows paths, unquoted unescaped spaces, bare names without `/` or extension,
// `~user`.

export interface FilePathMatch {
  /** Index of the first char of the match in the text. */
  start: number;
  /** Exclusive; covers the position suffix when present. */
  end: number;
  path: string;
  line: number | null;
  col: number | null;
}

export const MAX_MATCHES_PER_LINE = 20;
const MAX_PATH_LEN = 1024;
const MAX_POSITION_DIGITS = 7;

// Breaks: whitespace, ASCII quotes and brackets, box drawing/blocks/geometric shapes, curly quotes,
// guillemets, ellipsis, and the Claude Code framing glyphs U+23BF and U+23FA.
const TOKEN_BREAK = /[\s'"`()<>[\]{},;|\u2500-\u25FF\u2018-\u201F\u00AB\u00BB\u2026\u23BF\u23FA]/;
const TRAILING_PUNCT = /[.,:;!?\u2026]+$/;
const COLON_POSITION = /^(.*?):(\d+)(?::(\d+))?(?:-(\d+))?$/;
const HASH_POSITION = /^(.*?)#L(\d+)(?:C(\d+))?(?:-L?(\d+)(?:C\d+)?)?$/;
const PAREN_POSITION = /^\((\d+)(?:,\s*(\d+))?\)/;
const PYTHON_POSITION = /^["'`], line (\d+)/;
const EXTENSION = /\.[A-Za-z0-9]{1,10}$/;
const QUOTES = new Set(['"', "'", '`']);
const WORD = /[\p{L}\p{N}_]/u;

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

// Only an empty host or `localhost` names this machine.
function fileUrlPath(url: string): string | null {
  const rest = url.slice('file://'.length).replace(/^localhost(?=\/)/, '');
  if (!rest.startsWith('/')) return null;
  try {
    return decodeURIComponent(rest);
  } catch {
    return null;
  }
}

function parse(raw: string, start: number): FilePathMatch | null {
  const token = raw.replace(TRAILING_PUNCT, '');
  let rawPath = token;
  let line: number | null = null;
  let col: number | null = null;
  let end = start + token.length;
  const m = HASH_POSITION.exec(token) ?? COLON_POSITION.exec(token);
  if (m) {
    rawPath = m[1] ?? '';
    const l = position(m[2] ?? '');
    const c = m[3] === undefined ? null : position(m[3]);
    if (l !== null && (m[3] === undefined || c !== null)) {
      line = l;
      col = c;
    } else {
      end = start + rawPath.length;
    }
  }
  let path = rawPath.replace(/\\ /g, ' ');
  if (rawPath.startsWith('file://')) {
    const decoded = fileUrlPath(rawPath);
    if (decoded === null) return null;
    path = decoded;
  }
  if (!qualifies(path)) return null;
  return { start, end, path, line, col };
}

// Position written after the path: tsc `(12,5)`, Python `", line 12`.
function withTrailingPosition(m: FilePathMatch, text: string): FilePathMatch {
  if (m.line !== null) return m;
  const tail = text.slice(m.end, m.end + 40);
  const paren = PAREN_POSITION.exec(tail);
  if (paren) {
    const l = position(paren[1] ?? '');
    const c = paren[2] === undefined ? null : position(paren[2]);
    if (l !== null && (paren[2] === undefined || c !== null)) {
      return { ...m, end: m.end + paren[0].length, line: l, col: c };
    }
    return m;
  }
  const py = PYTHON_POSITION.exec(tail);
  const l = py ? position(py[1] ?? '') : null;
  return l === null ? m : { ...m, line: l };
}

// A quote opens a span only after a non-word char and closes before one, so `don't ... it's` stays
// two plain breaks.
function quotedSpan(text: string, i: number): { match: FilePathMatch; next: number } | null {
  const q = text.charAt(i);
  if (i > 0 && WORD.test(text.charAt(i - 1))) return null;
  const close = text.indexOf(q, i + 1);
  if (close < 0 || close - i > MAX_PATH_LEN + 1) return null;
  if (close + 1 < text.length && WORD.test(text.charAt(close + 1))) return null;
  const content = text.slice(i + 1, close);
  if (!content.includes(' ')) return null;
  const match = parse(content, i + 1);
  return match ? { match: withTrailingPosition(match, text), next: close + 1 } : null;
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
    if (QUOTES.has(ch)) {
      const span = quotedSpan(text, i);
      if (span) {
        out.push(span.match);
        i = span.next;
        continue;
      }
    }
    if (TOKEN_BREAK.test(ch)) {
      i++;
      continue;
    }
    let j = i;
    while (j < text.length && !TOKEN_BREAK.test(text.charAt(j))) {
      j += text.charAt(j) === '\\' && text.charAt(j + 1) === ' ' ? 2 : 1;
    }
    const raw = text.slice(i, j);
    // A token cut out of a web link (`https://h/a,b/c.go` splits at the comma) is not a path.
    const inWebLink = text.slice(spanStart, i).includes('://');
    if (raw.startsWith('file://') || !inWebLink) {
      const match = parse(raw, i);
      if (match) out.push(withTrailingPosition(match, text));
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
