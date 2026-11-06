import type { ConnectionKind } from '@shared/domain/connection';
import { MONGO_CONSOLE_METHODS } from '@shared/domain/console';
import {
  decodePath,
  encodePath,
  type PathSegment,
  type RelationColumns,
} from '@shared/domain/tree';
import type { EditorCompletionSource } from '../../editor/completion';
import { rowKey, useTreeStore } from '../../project/state/tree';
import { useMongoFieldSampleStore } from '../shared/mongoFieldSample';
import {
  MONGO_QUERY_OPERATORS,
  MONGO_VALUE_CONSTRUCTORS,
  type MongoValueConstructor,
} from '../shared/mongoVocabulary';
import { sqlDialectFor } from '../shared/sqlIdent';
import { type DdlSchema, EMPTY_DDL_SCHEMA } from './ddl';
import { sqlCompletionSources } from './sqlLanguageService';

// Reuses the same {insert, caretOffsetFromEnd} vocabulary the filter bar's plain AutocompleteField
// consumes — only the caret-positioning mechanism differs. Monaco's own snippet placeholder is `$0`
// (EditorCompletion.snippet's own doc comment, editor/completion.ts), the successor to CodeMirror's
// `#{}`.
function toSnippetTemplate(c: MongoValueConstructor): string {
  const pos = c.insert.length - c.caretOffsetFromEnd;
  return `${c.insert.slice(0, pos)}$0${c.insert.slice(pos)}`;
}

// The first path segment is the database node's own key
// (`database:<name>`, shared/domain/tree.ts's encodePath) — a console opened from a database or
// collection row always carries it; one opened from the connection root does not, and
// mongoCollectionNames degrades to an empty list rather than guessing.
function databaseSegment(path: string): string | null {
  const [first] = path.split('/');
  return first?.startsWith('database:') ? first : null;
}

// F5: reads the tree's own cache — no new round trip, no new cache. Empty (never expanded, or
// opened from the connection root) is the honest degradation the acceptance checklist names, not
// an error: the method/operator positions below don't depend on this at all.
function mongoCollectionNames(connectionId: string, path: string): string[] {
  const segment = databaseSegment(path);
  if (!segment) return [];
  const nodes = useTreeStore().children[rowKey(connectionId, segment)] ?? [];
  return nodes.filter((n) => n.kind === 'collection').map((n) => n.name);
}

const RELATION_CONTAINER_KINDS = new Set(['database', 'schema']);

const RELATION_NODE_KINDS = new Set(['table', 'view', 'matview']);

// mongoCollectionNames' own technique, carried to SQL — reads the tree's own cache, no
// new round trip, no new cache, an empty list is the honest degradation. Walks the console's own
// path back to the last database:/schema: segment (a table/view/matview console keeps its parent
// container in its own path; a console opened on the container itself already ends there) and
// reads whatever that row's own children already are. project/state/tree.ts's own finding
// means this can never see 'column' nodes (they moved into the definition view), only relation
// names. A console opened from the connection root has no such segment — rootRelationNames
// below is its own branch, not this function degrading to [].
export function consoleRelationNames(connectionId: string, path: string): string[] {
  let segments: PathSegment[];
  try {
    segments = decodePath(connectionId, path).segments;
  } catch {
    return [];
  }
  let cut = -1;
  for (let i = segments.length - 1; i >= 0; i--) {
    if (RELATION_CONTAINER_KINDS.has(segments[i]?.kind ?? '')) {
      cut = i;
      break;
    }
  }
  if (cut < 0) return path === '' ? rootRelationNames(connectionId) : [];
  const containerPath = encodePath(segments.slice(0, cut + 1));
  const nodes = useTreeStore().children[rowKey(connectionId, containerPath)] ?? [];
  return nodes.filter((n) => RELATION_NODE_KINDS.has(n.kind)).map((n) => n.name);
}

/** The root has no single database:/schema: container to scope relation names to (a
 *  connection can have many) — schemaColumns.ts's rootContainerPathFor picks one for the cached-
 *  columns supply, but this is the wider, zero-I/O fallback: the union of relation names across
 *  EVERY container already loaded for this connection in the tree cache, deduped. Strictly a
 *  superset of what a root console offered before this fix (nothing), and it's what makes a table
 *  the user already expanded in the tree show up in a root console's completion even when
 *  rootContainerPathFor can't pick a single container to cache columns for. */
function rootRelationNames(connectionId: string): string[] {
  const treeStore = useTreeStore();
  const prefix = rowKey(connectionId, '');
  const names = new Set<string>();
  for (const key of Object.keys(treeStore.children)) {
    if (!key.startsWith(prefix)) continue;
    for (const node of treeStore.children[key] ?? []) {
      if (RELATION_NODE_KINDS.has(node.kind)) names.add(node.name);
    }
  }
  return [...names];
}

// The last db.<collection>.<method>( call before the cursor — the field-name position
// below (a filter/projection literal) is scoped to whatever collection that call addresses, the
// same "look at the text before" technique every other position in this function already uses.
// A heuristic, not a parse: good enough for the one-statement-per-console shape this grammar
// already assumes everywhere else (mongo/console.ts's own db.<collection>.<method>(<args>) rule).
const DB_METHOD_CALL_RE = /\bdb\.([A-Za-z_$][\w$]*)\.[A-Za-z_$][\w$]*\(/g;

function enclosingCollectionPath(path: string, before: string): string | null {
  const segment = databaseSegment(path);
  if (!segment) return null;
  DB_METHOD_CALL_RE.lastIndex = 0;
  let last: RegExpExecArray | null = null;
  for (let m = DB_METHOD_CALL_RE.exec(before); m; m = DB_METHOD_CALL_RE.exec(before)) last = m;
  if (!last) return null;
  return `${segment}/collection:${encodeURIComponent(last[1] as string)}`;
}

// Four contextual positions, each independent of the others — offering a method the engine
// will reject would be worse than offering nothing, so this stays exactly as narrow as
// mongo/console.ts's own grammar (db.<collection>.<method>(<args>)). A bare identifier inside a
// filter/projection literal offers field names sampled from whatever documents that collection's
// own document tabs have loaded (views/shared/mongoFieldSample.ts) — a sample, not a schema
// (a Mongo collection has none), merged with the BSON-constructor list this position already
// offered rather than replacing it.
function mongoCompletionSource(connectionId: string, path: string): EditorCompletionSource {
  return ({ doc, offset }) => {
    // Deliberately not gated on "word non-empty or explicit" the way a generic word-completion
    // source would be: the two `before`-anchored positions below (right after `db.` or
    // `db.<collection>.`) are exactly where the current word is empty — that emptiness is the
    // trigger, not a reason to bail.
    const wordMatch = /[$\w]*$/.exec(doc.slice(0, offset));
    const wordText = wordMatch ? wordMatch[0] : '';
    const from = offset - wordText.length;
    const before = doc.slice(0, from);

    if (/\bdb\.$/.test(before)) {
      const names = mongoCollectionNames(connectionId, path);
      if (names.length === 0) return null;
      return { from, options: names.map((label) => ({ label, type: 'variable' as const })) };
    }
    if (/\bdb\.[A-Za-z_$][\w$]*\.$/.test(before)) {
      return {
        from,
        options: MONGO_CONSOLE_METHODS.map((label) => ({ label, type: 'method' as const })),
      };
    }
    if (wordText.startsWith('$')) {
      return {
        from,
        options: MONGO_QUERY_OPERATORS.map((label) => ({ label, type: 'keyword' as const })),
      };
    }
    // The six BSON constructors — offered wherever a bare word starts, same as
    // any other identifier completion — plus, when this word sits inside a recognisable
    // db.<collection>.<method>(…) call, that collection's own sampled field names ahead of them.
    // Monaco's own default matching narrows the combined list as more is typed.
    if (/^[A-Za-z]/.test(wordText)) {
      const collectionPath = enclosingCollectionPath(path, before);
      const fields = collectionPath
        ? useMongoFieldSampleStore().mongoFieldNamesFor(connectionId, collectionPath)
        : [];
      return {
        from,
        options: [
          ...fields.map((label) => ({ label, type: 'property' as const })),
          ...MONGO_VALUE_CONSTRUCTORS.map((c) => ({
            label: c.name,
            snippet: toSnippetTemplate(c),
            type: 'function' as const,
          })),
        ],
      };
    }
    return null;
  };
}

interface RedisCommandInfo {
  name: string;
  hint: string;
}

// The arity hint (`detail`) is the part a user cannot remember — the command list
// itself is curated, not exhaustive (this app's own convention): common data-access commands
// across every type this app's KeyValueView renders (string/hash/list/set/zset) plus the handful
// of housekeeping commands every session needs.
const REDIS_COMMANDS: readonly RedisCommandInfo[] = [
  { name: 'GET', hint: 'GET key' },
  { name: 'SET', hint: 'SET key value' },
  { name: 'SETEX', hint: 'SETEX key seconds value' },
  { name: 'DEL', hint: 'DEL key [key ...]' },
  { name: 'EXISTS', hint: 'EXISTS key [key ...]' },
  { name: 'EXPIRE', hint: 'EXPIRE key seconds' },
  { name: 'TTL', hint: 'TTL key' },
  { name: 'KEYS', hint: 'KEYS pattern' },
  { name: 'SCAN', hint: 'SCAN cursor [MATCH pattern] [COUNT count]' },
  { name: 'TYPE', hint: 'TYPE key' },
  { name: 'PERSIST', hint: 'PERSIST key' },
  { name: 'RENAME', hint: 'RENAME key newkey' },
  { name: 'INCR', hint: 'INCR key' },
  { name: 'DECR', hint: 'DECR key' },
  { name: 'HGET', hint: 'HGET key field' },
  { name: 'HSET', hint: 'HSET key field value [field value ...]' },
  { name: 'HGETALL', hint: 'HGETALL key' },
  { name: 'HDEL', hint: 'HDEL key field [field ...]' },
  { name: 'LPUSH', hint: 'LPUSH key value [value ...]' },
  { name: 'RPUSH', hint: 'RPUSH key value [value ...]' },
  { name: 'LRANGE', hint: 'LRANGE key start stop' },
  { name: 'SADD', hint: 'SADD key member [member ...]' },
  { name: 'SMEMBERS', hint: 'SMEMBERS key' },
  { name: 'ZADD', hint: 'ZADD key score member [score member ...]' },
  { name: 'ZRANGE', hint: 'ZRANGE key start stop [WITHSCORES]' },
  { name: 'PING', hint: 'PING' },
];

// True only when everything since the start of the statement (or since the last `;`) is
// whitespace — the tokenizer position is derivable straight from the text before the caret, no
// parse needed. No key-name completion, no argument completion (D22, F5).
const REDIS_FIRST_TOKEN_RE = /(^|;)\s*$/;

function redisCompletionSource(): EditorCompletionSource {
  return ({ doc, offset, explicit }) => {
    const wordMatch = /[^\s;]*$/.exec(doc.slice(0, offset));
    const wordText = wordMatch ? wordMatch[0] : '';
    const from = offset - wordText.length;
    if (from === offset && !explicit) return null;
    const before = doc.slice(0, from);
    if (!REDIS_FIRST_TOKEN_RE.test(before)) return null;
    return {
      from,
      options: REDIS_COMMANDS.map((cmd) => ({
        label: cmd.name,
        detail: cmd.hint,
        type: 'keyword' as const,
      })),
    };
  };
}

/** For the five SQL kinds: undefined with no DDL document AND no cached columns for this
 *  connection (the console's `autocomplete` prop is what gates SQL completion generally, and
 *  with neither supply completion falls back to `relations`/`sqlKeywordCompletionSource` alone,
 *  never `undefined` outright unless all three are empty); the schema+keyword pair
 *  (sqlLanguageService.ts) once either exists. `cached` is the metadata cache's own
 *  columns for this console's container — a plain value already in memory (state/schemaColumns.ts,
 *  filled by the view's own lifecycle hook), never fetched from here. */
export function consoleCompletionSources(
  kind: ConnectionKind,
  connectionId: string | null,
  path: string,
  schema?: DdlSchema,
  cached?: readonly RelationColumns[],
): readonly EditorCompletionSource[] | undefined {
  if (kind === 'mongodb' && connectionId) return [mongoCompletionSource(connectionId, path)];
  if (kind === 'redis') return [redisCompletionSource()];
  const dialect = sqlDialectFor(kind);
  if (!dialect) return undefined;
  // P19 D14: layered now, not all-or-nothing — a DDL document (schema) still wins when one
  // exists, but a connection with none still gets table-name completion from the tree's own
  // cache (relations), the same technique mongoCompletionSource already uses for collections.
  const relations = connectionId ? consoleRelationNames(connectionId, path) : [];
  return sqlCompletionSources(dialect, schema ?? EMPTY_DDL_SCHEMA, relations, cached ?? []);
}
