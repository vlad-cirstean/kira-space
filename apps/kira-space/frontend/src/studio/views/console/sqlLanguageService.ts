import type { RelationColumns } from '@shared/domain/tree';
import type { EditorCompletion, EditorCompletionSource } from '../../editor/completion';
import { identNeedsQuoting, quoteIdent, type SqlDialect } from '../shared/sqlIdent';
import { type DdlSchema, namespaceFromCached, toSqlNamespace } from './ddl';
import { sqlKeywordCompletionSource } from './sqlKeywordCompletion';
import { RELATION_POSITION_RE, sqlSchemaCompletionSource } from './sqlSchemaCompletion';

// Why this is a "language service", not a language server.
//
// The SPEC row asks for "a SQL language server (completions, diagnostics, hovers)". Those three
// verbs are textDocument/completion, textDocument/publishDiagnostics and textDocument/hover — and
// this app's own `MonacoHost.vue` already exposes each one directly as a `completionSources`
// prop, a `lintSource` prop and a `hoverSource` prop. What an LSP buys a
// general-purpose editor is process isolation and editor-independence: one server binary serving
// many editors, keeping a heavy analysis off the UI thread. This app has exactly one editor, one
// renderer, and an analysis whose entire state is one parsed object — and the vendored Node
// runtime an LSP child process would need was deleted, while the shipped build deliberately
// opens no local TCP port for another process to reach (docs/ARCHITECTURE.md's Process model). So:
// three providers over one DdlSchema, composed through props the console already had
// (completionSources, lintSource, hoverSource) — the same providers an LSP would run, in-process,
// with none of the machinery a real one exists for. Don't re-litigate this from the SPEC's wording
// alone.

// A relation name needing quotes (case-sensitive, a reserved word) gets `insert` set to its
// quoted form, the same identNeedsQuoting/quoteIdent rule namespaceFromCached (ddl.ts) and
// views/grid/filterCompletion.ts's own column completions already follow — `label` stays bare so
// matching/highlighting still works against the plain name.
function relationCompletionSource(
  relations: readonly string[],
  dialect: SqlDialect,
): EditorCompletionSource {
  return ({ doc, offset }) => {
    if (relations.length === 0) return null;
    const word = /[\w."]*$/.exec(doc.slice(0, offset));
    const from = word ? offset - word[0].length : offset;
    const before = doc.slice(0, from);
    if (!RELATION_POSITION_RE.test(before)) return null;
    return {
      from,
      options: relations.map((label): EditorCompletion => {
        const completion: EditorCompletion = { label, type: 'class' };
        return identNeedsQuoting(dialect, label)
          ? { ...completion, insert: quoteIdent(dialect, label) }
          : completion;
      }),
    };
  };
}

/** Layered, not all-or-nothing. A DDL document (`schema`) still
 *  wins wholesale when one has any tables, with `relations` merged into its table names at a
 *  relation position (the schema source takes `relations` itself: the first non-null source wins).
 *  With no document, `cached` (the metadata cache's own columns for
 *  this console's container, state/schemaColumns.ts's cachedRelationsFor) fills in the identical
 *  schema-aware completion — table names, `table.` column completion, alias resolution — with no
 *  manual step, ALSO paired with `relations`: a root-opened console's cached container and its
 *  wider set of tree-loaded relation names (completion.ts's own root branch) don't have to be the
 *  same container, so a name the cache doesn't cover but the tree already does is not discarded.
 *  Only once BOTH `schema` and `cached` are empty does `relations` (consoleRelationNames —
 *  completion.ts's own tree-cache read, mirroring mongoCompletionSource's identical technique for
 *  collections) carry the whole load alone, paired with an explicit keyword source (`override`
 *  replaces language-data sources wholesale). All three empty is
 *  exactly today's `undefined` — deliberate, not a gap to patch by having the language service
 *  query the database itself: the "no introspection from the language layer" rule holds, and
 *  this file's diagnostics/hover providers read the same effective schema, so completion never
 *  disagrees with them about what the console knows. */
export function sqlCompletionSources(
  dialect: SqlDialect,
  schema: DdlSchema,
  relations: readonly string[] = [],
  cached: readonly RelationColumns[] = [],
): readonly EditorCompletionSource[] | undefined {
  if (schema.tables.length > 0) {
    return [
      sqlSchemaCompletionSource(dialect, toSqlNamespace(schema), relations),
      sqlKeywordCompletionSource(dialect),
    ];
  }
  if (cached.length > 0) {
    return [
      // Relations from a container OTHER than the cached one (a root-opened console) merge in.
      sqlSchemaCompletionSource(dialect, namespaceFromCached(cached, dialect), relations),
      sqlKeywordCompletionSource(dialect),
    ];
  }
  if (relations.length > 0) {
    return [relationCompletionSource(relations, dialect), sqlKeywordCompletionSource(dialect)];
  }
  return undefined;
}
