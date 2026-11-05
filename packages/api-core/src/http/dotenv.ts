// The bulk `.env`-format editor's own pure logic — serialize, parse, reconcile —
// a renderer-side authoring format with no Go twin: Go receives a normalized entry list
// (VariablesRepo.ApplyBulk), never `.env` text, so there is no second parser to drift. Pure and
// DOM-free, the same reason substitute.ts/transforms.ts are: a plain, unit-testable import.

import type { TextRef } from '../lib/textRef';

/** One existing row, as the bulk editor's own reconcile needs it — the secret flag decides D22's
 *  rule 3 (an untouched empty line never touches secret_value), and `id` is what a matched update
 *  keeps (and therefore its history). */
export interface EnvRow {
  id: string;
  name: string;
  value: string;
  isSecret: boolean;
  description: string;
}

/** One `KEY=VALUE` pair parseEnv found, in file order. `hasValue: false` is `KEY=` with nothing
 *  after the `=` — D22 rule 3's own trigger. */
export interface EnvEntry {
  name: string;
  value: string;
  hasValue: boolean;
  description: string;
}

export interface EnvParseError {
  /** 1-based, matching how an editor reports its own line numbers. */
  line: number;
  message: TextRef;
}

export interface EnvParseResult {
  entries: EnvEntry[];
  error: EnvParseError | null;
}

export interface EnvDiffAdd {
  name: string;
  value: string;
  description: string;
}

export interface EnvDiffUpdate {
  id: string;
  name: string;
  value: string;
  description: string;
  /** False for a secret row whose line carried no new value (D22 rule 3) but whose description
   *  changed — the summary can then say "1 updated" without implying the secret's value moved. */
  valueChanged: boolean;
}

export interface EnvDiffRemove {
  id: string;
  name: string;
}

export interface EnvDiff {
  added: EnvDiffAdd[];
  updated: EnvDiffUpdate[];
  removed: EnvDiffRemove[];
  /** True when the surviving rows' relative order differs from their current sort_order. */
  reordered: boolean;
  /** Rename warning trigger: a name that vanished and a name that appeared in the same
   *  diff is exactly what a rename typed into the editor looks like from the reconcile's point of
   *  view — there is no way to tell that apart from an unrelated delete-and-add, which is the
   *  entire reason this is a warning rather than an automatic rename. */
  hasRenameRisk: boolean;
}

// D21: the exact marker comment a secret row's own value line is preceded by — recognised on
// parse and dropped rather than becoming part of the next pair's description (never round-tripped
// as text a user typed, since the renderer never has a secret's plaintext to compare against, F3).
export const SECRET_MARKER =
  '# secret — the value is not shown here and is left unchanged; enter it in the variables list';

function needsQuoting(value: string): boolean {
  if (value === '') return true; // empty-but-not-secret — a bare `KEY=` would parse as hasValue: false
  if (value !== value.trim()) return true; // leading/trailing whitespace would be lost unquoted
  if (value.startsWith("'")) return true; // parseEnv would read it back as a single-quoted value
  return /[\n"#]/.test(value);
}

function escapeQuoted(value: string): string {
  // Order matters: backslashes first, or a later replacement's own backslash would be re-escaped.
  return value
    .replace(/\\/g, '\\\\')
    .replace(/"/g, '\\"')
    .replace(/\n/g, '\\n')
    .replace(/\t/g, '\\t');
}

function quoteIfNeeded(value: string): string {
  return needsQuoting(value) ? `"${escapeQuoted(value)}"` : value;
}

/**
 * Renders `rows` as `.env` text, one block per row in the given order — D21's own format:
 * - a non-empty description becomes `# `-prefixed comment lines above the pair, one per line of
 *   the description;
 * - a secret row emits the fixed marker comment and an **empty value** — the renderer never has
 *   the plaintext to write out (F3), so this is a property of the architecture, not a choice;
 * - a non-secret value is quoted (double, with `\n`/`\t`/`\\`/`"` escapes) iff it is empty, has
 *   leading/trailing whitespace, starts with `'`, or contains `\n`, `"` or `#`; otherwise emitted raw.
 */
export function serializeEnv(rows: readonly EnvRow[]): string {
  const blocks = rows.map((row) => {
    const lines: string[] = [];
    if (row.description !== '') {
      for (const line of row.description.split('\n')) lines.push(`# ${line}`);
    }
    if (row.isSecret) {
      lines.push(SECRET_MARKER);
      lines.push(`${row.name}=`);
    } else {
      lines.push(`${row.name}=${quoteIfNeeded(row.value)}`);
    }
    return lines.join('\n');
  });
  return blocks.length === 0 ? '' : `${blocks.join('\n\n')}\n`;
}

/** Scans a double-quoted value starting at `raw[0]`: the decoded text and the index just past the
 *  closing quote, or null when no unescaped closing quote exists. */
function scanDoubleQuoted(raw: string): { value: string; end: number } | null {
  let out = '';
  for (let i = 1; i < raw.length; i++) {
    const c = raw[i];
    if (c === '"') return { value: out, end: i + 1 };
    if (c !== '\\') {
      out += c;
      continue;
    }
    const next = raw[i + 1];
    if (next === undefined) return null;
    // Lenient: an unrecognised escape keeps its literal character.
    out += next === 'n' ? '\n' : next === 't' ? '\t' : next;
    i++;
  }
  return null;
}

/** A quoted value only counts when the closing quote ends the line, or is followed by whitespace
 *  and an optional `# comment`; anything else (no close, trailing junk) is an unquoted value. */
function scanQuoted(raw: string): string | null {
  const quote = raw[0];
  let scanned: { value: string; end: number } | null;
  if (quote === '"') {
    scanned = scanDoubleQuoted(raw);
  } else if (quote === "'") {
    const close = raw.indexOf("'", 1);
    scanned = close === -1 ? null : { value: raw.slice(1, close), end: close + 1 };
  } else {
    return null;
  }
  if (!scanned) return null;
  const after = raw.slice(scanned.end);
  return after.trim() === '' || /^\s+#/.test(after) ? scanned.value : null;
}

/**
 * Parses `.env`-format text into an ordered entry list — the eight line forms:
 * a blank line (ends the pending comment block), a `# text` comment (accumulates into the next
 * pair's description), the exact secret marker (dropped, never a description), an `export `-
 * prefixed pair (the prefix is stripped), an unquoted/double-quoted/single-quoted value, and a
 * bare `KEY=` (`hasValue: false`). Anything else is a parse error carrying its 1-based line
 * number — line-oriented and single-pass, so a caller can stop at the first bad line.
 *
 * Value rules follow common dotenv implementations: an unquoted value ends at a `#`
 * preceded by whitespace (`a#b` keeps its `#`) and is trimmed; a quoted value needs a closing quote
 * on the same line followed by nothing or a `# comment`, else the whole text is an unquoted value
 * (`'abc` stays `'abc`, never an error). Single quotes are literal; double quotes decode escapes.
 */
export function parseEnv(text: string): EnvParseResult {
  const lines = text.split('\n');
  const entries: EnvEntry[] = [];
  let pendingDescription: string[] = [];

  for (let i = 0; i < lines.length; i++) {
    const rawLine = lines[i].replace(/\r$/, '');
    const lineNo = i + 1;
    const trimmed = rawLine.trim();

    if (trimmed === '') {
      pendingDescription = [];
      continue;
    }
    if (trimmed === SECRET_MARKER) {
      continue;
    }
    if (trimmed.startsWith('#')) {
      const rest = trimmed.slice(1);
      pendingDescription.push(rest.startsWith(' ') ? rest.slice(1) : rest);
      continue;
    }

    const content = trimmed.startsWith('export ') ? trimmed.slice('export '.length) : trimmed;
    const eq = content.indexOf('=');
    if (eq === -1) {
      return {
        entries,
        error: {
          line: lineNo,
          message: {
            key: 'workbench.apiImport.dotenvNoEquals.error',
            params: { line: lineNo, example: 'KEY=VALUE' },
          },
        },
      };
    }
    const key = content.slice(0, eq).trim();
    const rawValue = content.slice(eq + 1);
    if (key === '') {
      return {
        entries,
        error: {
          line: lineNo,
          message: {
            key: 'workbench.apiImport.dotenvNoKey.error',
            params: { line: lineNo, sign: '=' },
          },
        },
      };
    }

    const description = pendingDescription.join('\n');
    pendingDescription = [];

    const quoted = scanQuoted(rawValue.trimStart());
    if (quoted !== null) {
      entries.push({ name: key, value: quoted, hasValue: true, description });
      continue;
    }
    const commentAt = rawValue.search(/\s#/);
    const value = (commentAt === -1 ? rawValue : rawValue.slice(0, commentAt)).trim();
    entries.push({ name: key, value, hasValue: value !== '', description });
  }

  return { entries, error: null };
}

/**
 * Diffs `parsed` (a fresh parseEnv result) against `existing` (the rows the editor opened with) —
 * the five rules, matching by name and positionally for a duplicate key (the Nth occurrence of a
 * name maps to the Nth existing row with that name, in `existing`'s own order). Pure and read-only:
 * this never talks to Go — VariablesRepo.ApplyBulk reconciles independently server-side from
 * the same parsed entries, and the two reconciles are expected to agree.
 */
export function reconcileEnv(existing: readonly EnvRow[], parsed: readonly EnvEntry[]): EnvDiff {
  const pools = new Map<string, EnvRow[]>();
  for (const row of existing) {
    const list = pools.get(row.name);
    if (list) list.push(row);
    else pools.set(row.name, [row]);
  }
  const nextIndex = new Map<string, number>();

  const added: EnvDiffAdd[] = [];
  const updated: EnvDiffUpdate[] = [];
  const matchedIds = new Set<string>();
  const matchedOrder: string[] = [];

  for (const entry of parsed) {
    const pool = pools.get(entry.name);
    const idx = nextIndex.get(entry.name) ?? 0;
    const row = pool?.[idx];
    if (!row) {
      added.push({ name: entry.name, value: entry.value, description: entry.description });
      continue;
    }
    nextIndex.set(entry.name, idx + 1);
    matchedIds.add(row.id);
    matchedOrder.push(row.id);

    if (row.isSecret) {
      // D22 rule 3: an untouched line (hasValue: false) never touches secret_value — only a
      // description change (if any) is a real update.
      if (entry.hasValue) {
        updated.push({
          id: row.id,
          name: entry.name,
          value: entry.value,
          description: entry.description,
          valueChanged: true,
        });
      } else if (entry.description !== row.description) {
        updated.push({
          id: row.id,
          name: entry.name,
          value: row.value,
          description: entry.description,
          valueChanged: false,
        });
      }
      continue;
    }

    const valueChanged = entry.value !== row.value;
    const descriptionChanged = entry.description !== row.description;
    if (valueChanged || descriptionChanged) {
      updated.push({
        id: row.id,
        name: entry.name,
        value: entry.value,
        description: entry.description,
        valueChanged,
      });
    }
  }

  const removed: EnvDiffRemove[] = existing
    .filter((row) => !matchedIds.has(row.id))
    .map((row) => ({ id: row.id, name: row.name }));

  // D22 rule 5: line order becomes sort_order — a pure reorder is "the surviving rows' relative
  // order changed", independent of any add/remove.
  const priorOrder = existing.filter((row) => matchedIds.has(row.id)).map((row) => row.id);
  const reordered = priorOrder.join(' ') !== matchedOrder.join(' ');

  return {
    added,
    updated,
    removed,
    reordered,
    hasRenameRisk: added.length > 0 && removed.length > 0,
  };
}
