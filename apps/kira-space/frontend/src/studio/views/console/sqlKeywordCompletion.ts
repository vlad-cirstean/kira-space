// The successor to `@codemirror/lang-sql`'s `keywordCompletionSource(dialect, true)` —
// every dialect's keywords and type names, uppercased (`editor/languages.ts`'s now-removed
// `upperCaseKeywords: true`, whose own reasoning stays true: house style, and Monaco's own
// suggest-widget matcher case-folds the same way CodeMirror's `FuzzyMatcher` did, so typing `sel`
// still matches `SELECT`).
import { keywordsFor, typesFor } from '@shared/domain/sqlKeywords';
import type { EditorCompletion, EditorCompletionSource } from '../../editor/completion';
import type { SqlDialect } from '../shared/sqlIdent';

const optionsCache = new Map<SqlDialect, readonly EditorCompletion[]>();

function buildOptions(dialect: SqlDialect): readonly EditorCompletion[] {
  const keywordOptions = [...keywordsFor(dialect)].map(
    (w): EditorCompletion => ({ label: w.toUpperCase(), type: 'keyword' }),
  );
  const typeOptions = [...typesFor(dialect)].map(
    (w): EditorCompletion => ({ label: w.toUpperCase(), type: 'keyword' }),
  );
  // A type name and a keyword can collide (e.g. `array`, `json` are both a keyword and a common
  // type in more than one dialect) — dedupe by label, keyword wins (both render identically here,
  // `type: 'keyword'`, so which one survives is immaterial beyond avoiding a literal duplicate
  // entry in the popup).
  const byLabel = new Map<string, EditorCompletion>();
  for (const opt of [...keywordOptions, ...typeOptions]) {
    if (!byLabel.has(opt.label)) byLabel.set(opt.label, opt);
  }
  return [...byLabel.values()];
}

function optionsFor(dialect: SqlDialect): readonly EditorCompletion[] {
  let cached = optionsCache.get(dialect);
  if (!cached) {
    cached = buildOptions(dialect);
    optionsCache.set(dialect, cached);
  }
  return cached;
}

const WORD_RE = /[A-Za-z_][\w$]*$/;

/** Fires at a bare-word position — the same "word.from === word.to && !explicit -> no popup" rule
 *  `@codemirror/autocomplete`'s own `completeFromList` helper (what `keywordCompletionSource` was
 *  built on) followed, so an untyped bare cursor doesn't pop this up on every trigger character. */
export function sqlKeywordCompletionSource(dialect: SqlDialect): EditorCompletionSource {
  const options = optionsFor(dialect);
  return ({ doc, offset, explicit }) => {
    const before = doc.slice(0, offset);
    const match = WORD_RE.exec(before);
    const from = match ? offset - match[0].length : offset;
    if (from === offset && !explicit) return null;
    return { from, options };
  };
}
