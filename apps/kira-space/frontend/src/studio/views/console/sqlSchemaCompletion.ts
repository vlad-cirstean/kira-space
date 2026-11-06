// The successor to `@codemirror/lang-sql`'s `schemaCompletionSource` — what the library
// gave, as this app configured it: table names at an identifier position, `table.`/`alias.` column
// completion, alias resolution within the current statement, qualified `schema.table.column`
// paths, and a `defaultSchema` that flattens one level.
//
// Rebuilt on machinery this repo already owns, and better connected than today: `sqlRefs.ts`'s
// `statementsWithRefs` already resolves FROM/JOIN refs, aliases and CTE names for
// diagnostics/hover — that file's own header records it was written from scratch *because*
// lang-sql's own `getAliases` was not reusable. Before this phase the console had **two** alias
// resolvers that could disagree (lang-sql's, for completion; this repo's, for hover/diagnostics).
// After this there is one, shared by all three.
import type { EditorCompletion, EditorCompletionSource } from '../../editor/completion';
import { identNeedsQuoting, quoteIdent, type SqlDialect } from '../shared/sqlIdent';
import type { SchemaNamespace } from './ddl';
import { sqlKeywordCompletionSource } from './sqlKeywordCompletion';
import { unquoteIdent } from './sqlNodes';
import { statementsWithRefs } from './sqlRefs';

// A qualifier immediately before the cursor: `<name-or-"quoted">.<partial>` — the same
// "plain regex over doc.slice(0, offset)" technique `relationCompletionSource`/
// `mongoCompletionSource` (`completion.ts`) already use for their own trigger positions.
const QUALIFIED_RE =
  /("(?:[^"]|"")*"|`(?:[^`]|``)*`|\[(?:[^\]]|\]\])*\]|[A-Za-z_][\w$]*)\.([\w$]*)$/;
// P19 D14: fires only right after FROM/JOIN/UPDATE/INTO/TABLE; a column list or WHERE clause gets no
// table-name noise.
export const RELATION_POSITION_RE = /\b(from|join|update|into|table)\s+$/i;
const BARE_WORD_RE = /[\w$]*$/;

function toOption(dialect: SqlDialect, entry: EditorCompletion): EditorCompletion {
  if (entry.insert !== undefined) return entry;
  return identNeedsQuoting(dialect, entry.label)
    ? { ...entry, insert: quoteIdent(dialect, entry.label) }
    : entry;
}

function columnsAt(
  namespace: SchemaNamespace,
  name: string,
): readonly EditorCompletion[] | undefined {
  const entry = namespace[name];
  return Array.isArray(entry) ? entry : undefined;
}

function schemaAt(namespace: SchemaNamespace, name: string): SchemaNamespace | undefined {
  const entry = namespace[name];
  return entry !== undefined && !Array.isArray(entry) ? (entry as SchemaNamespace) : undefined;
}

// Position 1: `<qualifier>.` where `qualifier` resolves via `statementsWithRefs` to a table/alias
// referenced in the current statement (and is not a CTE name — the same "false positive worse
// than missing" guard `sqlDiagnostics.ts`/`sqlHover.ts` already apply to alias resolution).
function aliasColumns(
  dialect: SqlDialect,
  namespace: SchemaNamespace,
  doc: string,
  offset: number,
  qualifier: string,
): readonly EditorCompletion[] | undefined {
  const statement = statementsWithRefs(dialect, doc).find(
    (s) => s.statement.from <= offset && offset <= s.statement.to,
  );
  if (!statement) return undefined;
  const ref = statement.refs.find(
    (r) => (r.alias ?? r.name).toLowerCase() === qualifier.toLowerCase(),
  );
  if (!ref || statement.cteNames.has(ref.name.toLowerCase())) return undefined;
  if (ref.schema) {
    const schemaNs = schemaAt(namespace, ref.schema);
    const nested = schemaNs && columnsAt(schemaNs, ref.name);
    if (nested) return nested;
  }
  return columnsAt(namespace, ref.name);
}

/** `schemaCompletionSource` + `keywordCompletionSource(dialect, true)`, merged into one
 *  source. Positions, in order: `<qualifier>.` resolved via an alias/table reference in the
 *  current statement; `<schema>.` for a known schema qualifier; a bare word, which offers table
 *  names **and** keyword/type completions — merged here, not left to a separately-composed
 *  keyword source, since `MonacoHost.vue`'s own completion provider returns the first
 *  source in `completionSources` whose result is non-null rather than merging every source's
 *  options together the way `@codemirror/autocomplete`'s `override` array did.
 *
 *  No `defaultSchema` parameter (unlike lang-sql's own `schemaCompletionSource`): `ddl.ts`'s
 *  `toSqlNamespace`/`namespaceFromCached` already flatten every qualified table to a bare top-level
 *  key unconditionally (their own doc comments), so a default-schema table is already reachable
 *  unqualified with nothing extra needed here. */
export function sqlSchemaCompletionSource(
  dialect: SqlDialect,
  namespace: SchemaNamespace,
  relations: readonly string[] = [],
): EditorCompletionSource {
  const keywordSource = sqlKeywordCompletionSource(dialect);
  return (ctx) => {
    const before = ctx.doc.slice(0, ctx.offset);
    const qualifiedMatch = QUALIFIED_RE.exec(before);
    if (qualifiedMatch) {
      const qualifierRaw = qualifiedMatch[1] as string;
      const partial = qualifiedMatch[2] as string;
      const qualifier = unquoteIdent(qualifierRaw);
      const from = ctx.offset - partial.length;

      const aliasCols = aliasColumns(dialect, namespace, ctx.doc, ctx.offset, qualifier);
      if (aliasCols) return { from, options: aliasCols.map((c) => toOption(dialect, c)) };

      // Position 2: `<schema>.` — a known schema qualifier's own tables. `defaultSchema`'s own
      // flattened level (ddl.ts's defaultSchemaFor) is already reachable as bare table names via
      // the flat namespace entries `toSqlNamespace`/`namespaceFromCached` build, so nothing extra
      // is needed here for it.
      const schemaNs = schemaAt(namespace, qualifier);
      if (schemaNs) {
        const options: EditorCompletion[] = Object.keys(schemaNs).map((name) =>
          toOption(dialect, { label: name, type: 'class' }),
        );
        return { from, options };
      }
      return null;
    }

    // Position 4: a bare word — every top-level namespace key (table or nested schema name) plus
    // the dialect's own keyword/type vocabulary.
    const wordMatch = BARE_WORD_RE.exec(before);
    const word = wordMatch ? wordMatch[0] : '';
    const from = ctx.offset - word.length;
    if (from === ctx.offset && !ctx.explicit) return null;

    const tableOptions: EditorCompletion[] = Object.keys(namespace).map((name) =>
      toOption(dialect, { label: name, type: 'class' }),
    );
    // Tree-cache relations outside the namespace (other containers of a root-opened console): the
    // first non-null source wins, so they must be merged here, not left to a separate source.
    if (relations.length > 0 && RELATION_POSITION_RE.test(before.slice(0, from))) {
      const known = new Set(Object.keys(namespace).map((n) => n.toLowerCase()));
      for (const name of relations) {
        if (known.has(name.toLowerCase())) continue;
        known.add(name.toLowerCase());
        tableOptions.push(toOption(dialect, { label: name, type: 'class' }));
      }
    }
    const keywordResult = keywordSource(ctx);
    const keywordOptions = keywordResult?.options ?? [];
    return { from, options: [...tableOptions, ...keywordOptions] };
  };
}
