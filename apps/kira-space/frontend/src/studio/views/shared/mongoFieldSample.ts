// Candidate lists for DocumentView.vue's filter/SORT boxes, alongside a second job: sampling field
// names for the Mongo *console* too. Both jobs read
// the same idea — "field names sampled from documents actually loaded" — but through two different
// doors: the filter/sort box functions below take an explicit `fieldNames` list a caller already
// has (DocumentView.vue's own fieldNamesOnPage(tabId), a per-tab read); the console has no page of
// its own to read, so `registerMongoFieldSample`/`mongoFieldNamesFor` is a small, connection-scoped
// store DocumentView.vue pushes into whenever its own page changes, keyed by
// (connectionId, collectionPath) — never by tabId, so a console opened on the same collection from
// a different tab still sees it. Mandatory promotion: biome.json forbids views/console/**
// importing views/documents/** (the same rule clipboardFormats.ts was promoted for), and the
// console needs exactly this.
//
// This is a SAMPLE, not a schema — stated plainly here: it offers what has been
// seen, degrades to nothing when nothing has been loaded, and is never cached server-side (a Mongo
// collection has no declared field set to cache, adapters/mongo/adapter.go's own Columns: [] for
// the identical reason).

import type { Completion } from '@workbench/editor/fieldCompletion';
import { defineStore } from 'pinia';
import { reactive } from 'vue';
import { rowKey } from '../../project/state/tree';
import { MONGO_QUERY_OPERATORS, MONGO_VALUE_CONSTRUCTORS } from './mongoVocabulary';

// Mirrors engine/adapters/mongo/literal.ts's own bare-identifier tokenizer rule exactly — a field
// outside it must be quoted to parse as a filter/sort-document key.
const BARE_SAFE_RE = /^[A-Za-z_$][A-Za-z0-9_$]*$/;

// The trailing ": " is the one place an insertion usefully exceeds its label — a filter/sort
// document is always `key: value`.
function fieldInsertion(name: string): string {
  return BARE_SAFE_RE.test(name) ? `${name}: ` : `'${name.replace(/'/g, "\\'")}': `;
}

function fieldCompletions(fieldNames: readonly string[]): Completion[] {
  return fieldNames.map((name) => ({
    label: name,
    insert: fieldInsertion(name),
    detail: 'field',
    icon: 'symbol-field',
  }));
}

/** `fieldNames` deliberately excludes/includes `_id` at the caller's discretion —
 *  DocumentView.vue's own filterCandidates prepends it ("it is always returned regardless of
 *  projection ... so it is never a real projection choice" — correct for the projection picker
 *  fieldNamesOnPage was written for, wrong for a filter box). */
export function mongoFilterCandidates(fieldNames: readonly string[]): Completion[] {
  const operators = MONGO_QUERY_OPERATORS.map((label) => ({
    label,
    detail: 'operator',
    icon: 'symbol-operator',
  }));
  // P27 D17: the six BSON constructors, only where a value goes — never offered for the sort
  // box below (mongoSortCandidates), which has no use for one.
  const constructors = MONGO_VALUE_CONSTRUCTORS.map((c) => ({
    label: c.name,
    insert: c.insert,
    caretOffsetFromEnd: c.caretOffsetFromEnd,
    detail: 'constructor',
    icon: 'symbol-constructor',
  }));
  return [...fieldCompletions(fieldNames), ...operators, ...constructors];
}

// The sort box's own key: value grammar (DocumentView.vue's parseSortText) supplies the rest —
// bare field names only, no operators (a sort document has no use for one).
export function mongoSortCandidates(fieldNames: readonly string[]): Completion[] {
  return fieldCompletions(fieldNames);
}

export const useMongoFieldSampleStore = defineStore('mongoFieldSample', () => {
  // connectionId + the collection's own tree path (e.g. "database:db/collection:widgets")
  // -> every field name any document tab for that collection has loaded, unioned across tabs.
  // rowKey (project/state/tree.ts) is reused verbatim for the key shape, not because this is tree
  // state, but so the two can never disagree about what "this collection" means. Returned as a
  // named property directly (not toRefs) — a Record with dynamic keys.
  const byCollection = reactive<Record<string, string[]>>({});

  /** Called by DocumentView.vue whenever its own loaded page changes — never by a CompletionSource,
   *  mirroring the language layer's own "receives data, never fetches" rule (P22c D5): this is a
   *  push from a view that already has the data, not a pull that could trigger one. */
  function registerMongoFieldSample(
    connectionId: string,
    collectionPath: string,
    fieldNames: readonly string[],
  ): void {
    byCollection[rowKey(connectionId, collectionPath)] = [...fieldNames];
  }

  /** A plain property lookup — [] when nothing has been loaded yet, the honest degradation D8 names
   *  rather than an error. */
  function mongoFieldNamesFor(connectionId: string, collectionPath: string): readonly string[] {
    return byCollection[rowKey(connectionId, collectionPath)] ?? [];
  }

  return { byCollection, registerMongoFieldSample, mongoFieldNamesFor };
});
