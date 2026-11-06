import type { MutationRowOp } from '@shared/domain/mutations';
import { newUuid } from '@shared/ids';
import type { MutateResponse } from '@shared/protocol/dataOps';
import { cellText, isNull, type TabularPage } from '@shared/protocol/page';
import { compareNatural } from '@workbench/behaviour/match';
import { defineStore } from 'pinia';
import { reactive, toRaw } from 'vue';
import { data } from '../../bridge/data';
import { nullCellText } from '../shared/page/cellText';
import { cell, getPage, pageVersion } from './page';

// Renderer-only, in-memory, per tab — never persisted (not tabs.state_json, not settings,
// not a new SQLite table). Closing a tab discards uncommitted edits, exactly like closing a
// spreadsheet you never saved. A staged edit or delete is keyed by its row's primary key, not its
// page position, so it survives paging, sort, filter, projection and Refresh; a page that no
// longer holds the row just shows nothing for it until the row comes back on a page.

type RowKey = Record<string, string | null>;

interface PendingEdit {
  key: RowKey; // the row's primary key when it was staged — a PK-column edit still addresses the original row
  changes: Record<string, string | null>;
  original: Record<string, string | null>; // page value per column when first staged; edits off the current page keep it
}

export interface PendingCellDiff {
  id: string;
  keyLabel: string;
  column: string;
  before: string | null;
  after: string | null;
}

interface PendingDelete {
  key: RowKey;
}

interface PendingInsert {
  id: string; // local identity for Vue :key and discard — never sent to the server
  values: Record<string, string | null>;
}

export interface TabPending {
  edits: Map<string, PendingEdit>; // canonical row key (rowKeyId) -> edit
  deletes: Map<string, PendingDelete>;
  inserts: PendingInsert[];
}

interface RowKeyEntry {
  id: string;
  key: RowKey;
}

// Row -> key memo and key -> row index, per loaded page object (a new page naturally drops both).
// Non-reactive by design: the cell extractor reads them per cell during SlickGrid's own render.
interface PageKeys {
  pkSignature: string;
  ids: (RowKeyEntry | null | undefined)[];
  rowById?: Map<string, number>;
}
const pageKeys = new WeakMap<TabularPage, PageKeys>();
const keyDecoder = new TextDecoder();

// D8: delete, then update, then insert — mirrors the adapter's own execution order so the
// *Preview command* panel shows exactly what mutate() will run.

export const usePendingChangesStore = defineStore('pendingChanges', () => {
  const pendingState = reactive({} as Record<string, TabPending>);
  const committingState = reactive({} as Record<string, true>);

  function ensure(tabId: string): TabPending {
    if (!pendingState[tabId]) {
      pendingState[tabId] = { edits: new Map(), deletes: new Map(), inserts: [] };
    }
    return pendingState[tabId];
  }

  function pendingFor(tabId: string): TabPending | undefined {
    return pendingState[tabId];
  }

  // SlickGridHost.vue's own header states the rule — "Vue must not see the grid" — because the cell
  // extractor runs *during* SlickGrid's own synchronous render, not Vue's. Reading pending state
  // through pendingFor() breaks that rule in the one place that runs per cell:
  // pendingState[tabId] is itself a reactive proxy, its `edits` a reactive-wrapped Map, so
  // `pendingFor(tabId)?.edits.get(row)?.changes[column]` was up to four proxy traps (record,
  // TabPending, the Map collection handler's own toRaw-and-rewrap, then `changes`) for information
  // that cannot change mid-render. toRaw returns the same underlying object reactive() wraps —
  // nothing here is cloned, so this stays live as of whenever the caller took the snapshot; callers
  // that read it many times per render (dataSource.ts's cell extractor) should snapshot once via
  // this function rather than calling pendingFor() itself inside a per-cell hot loop.
  function rawPendingFor(tabId: string): TabPending | undefined {
    const p = pendingState[tabId];
    return p ? toRaw(p) : undefined;
  }

  function hasPending(tabId: string): boolean {
    const p = pendingState[tabId];
    return !!p && (p.edits.size > 0 || p.deletes.size > 0 || p.inserts.length > 0);
  }

  function clearPending(tabId: string): void {
    delete pendingState[tabId];
  }

  // state.ts registers an accessor for a tab's *full* primary-key column list (rt.meta.primaryKey,
  // loaded once per tab via treeDescribe) — importing its `runtime` back from here would be a
  // cycle, the same registry-inversion shape state/viewCommands.ts uses. null until state.ts has
  // run or while a tab's meta hasn't loaded yet: a row is then addressable by whichever PK columns
  // the page carries.
  let fullPrimaryKeyOf: ((tabId: string) => string[] | null) | null = null;
  function registerFullPrimaryKeyAccessor(fn: (tabId: string) => string[] | null): void {
    fullPrimaryKeyOf = fn;
  }

  function computeRowKey(
    page: TabularPage,
    full: string[] | null,
    row: number,
  ): RowKeyEntry | null {
    const key: RowKey = {};
    for (let col = 0; col < page.columns.length; col++) {
      const descriptor = page.columns[col];
      if (!descriptor.isPrimaryKey) continue;
      const chunk = page.chunks[col];
      key[descriptor.name] = isNull(chunk, row) ? null : cellText(chunk, row, keyDecoder);
    }
    const names = Object.keys(key).sort(compareNatural);
    if (names.length === 0) return null;
    // A partial key (Hide column on part of a composite key) cannot address the row.
    if (full?.some((name) => !(name in key))) return null;
    return { id: JSON.stringify(names.map((n) => [n, key[n]])), key };
  }

  function keysFor(
    tabId: string,
  ): { page: TabularPage; keys: PageKeys; full: string[] | null } | null {
    const page = getPage(tabId);
    if (!page) return null;
    const full = fullPrimaryKeyOf?.(tabId) ?? null;
    const pkSignature = full ? full.join('\u0000') : '';
    let keys = pageKeys.get(page);
    if (!keys || keys.pkSignature !== pkSignature) {
      keys = { pkSignature, ids: new Array(page.rowCount) };
      pageKeys.set(page, keys);
    }
    return { page, keys, full };
  }

  /** Row identity for staging: the table's full primary key, as read from the current page.
   *  null when the page has no (complete) primary key — nothing can be staged against the row. */
  function rowKeyOf(tabId: string, row: number): RowKeyEntry | null {
    const ctx = keysFor(tabId);
    if (!ctx || row < 0 || row >= ctx.page.rowCount) return null;
    const cached = ctx.keys.ids[row];
    if (cached !== undefined) return cached;
    const entry = computeRowKey(ctx.page, ctx.full, row);
    ctx.keys.ids[row] = entry;
    return entry;
  }

  function rowsByKey(tabId: string): Map<string, number> {
    const ctx = keysFor(tabId);
    if (!ctx) return new Map();
    if (!ctx.keys.rowById) {
      const index = new Map<string, number>();
      for (let row = 0; row < ctx.page.rowCount; row++) {
        const entry = rowKeyOf(tabId, row);
        if (entry) index.set(entry.id, row);
      }
      ctx.keys.rowById = index;
    }
    return ctx.keys.rowById;
  }

  /** Whether every primary-key column of the table is present in the current page, so a row can
   *  be addressed. Before the table's meta loads this is "the page has some PK column". */
  function pageHasFullPrimaryKey(tabId: string): boolean {
    const page = getPage(tabId);
    if (!page) return false;
    const present = new Set(page.columns.filter((c) => c.isPrimaryKey).map((c) => c.name));
    if (present.size === 0) return false;
    const full = fullPrimaryKeyOf?.(tabId) ?? null;
    return full ? full.every((name) => present.has(name)) : true;
  }

  function isPendingDelete(tabId: string, row: number): boolean {
    const entry = rowKeyOf(tabId, row);
    return !!entry && (pendingState[tabId]?.deletes.has(entry.id) ?? false);
  }

  function stagedValue(tabId: string, row: number, column: string): string | null | undefined {
    const entry = rowKeyOf(tabId, row);
    return entry ? pendingState[tabId]?.edits.get(entry.id)?.changes[column] : undefined;
  }

  function stagedOriginal(tabId: string, row: number, column: string): string | null | undefined {
    const entry = rowKeyOf(tabId, row);
    const edit = entry ? pendingState[tabId]?.edits.get(entry.id) : undefined;
    return edit && column in edit.changes ? (edit.original[column] ?? null) : undefined;
  }

  // Non-reactive twins for dataSource.ts's per-cell/per-row hot path (see rawPendingFor above):
  // pending state is read through toRaw, and a tab with nothing staged returns before any key work.
  function rawStagedValue(tabId: string, row: number, column: string): string | null | undefined {
    const p = rawPendingFor(tabId);
    if (!p || p.edits.size === 0) return undefined;
    const entry = rowKeyOf(tabId, row);
    return entry ? p.edits.get(entry.id)?.changes[column] : undefined;
  }

  function rawRowChange(tabId: string, row: number): 'delete' | 'edit' | null {
    const p = rawPendingFor(tabId);
    if (!p || (p.edits.size === 0 && p.deletes.size === 0)) return null;
    const entry = rowKeyOf(tabId, row);
    if (!entry) return null;
    if (p.deletes.has(entry.id)) return 'delete';
    return p.edits.has(entry.id) ? 'edit' : null;
  }

  function hasRowChange(tabId: string, row: number): boolean {
    const entry = rowKeyOf(tabId, row);
    const p = pendingState[tabId];
    return !!entry && !!p && (p.edits.has(entry.id) || p.deletes.has(entry.id));
  }

  /** Staged edits and deletes that sit on the current page, as page rows. Reactive on both the
   *  staged set and the loaded page. */
  function pendingOnPage(tabId: string): {
    edits: Map<number, Record<string, string | null>>;
    deletes: Set<number>;
  } {
    void pageVersion.n;
    const edits = new Map<number, Record<string, string | null>>();
    const deletes = new Set<number>();
    const p = pendingState[tabId];
    if (!p || (p.edits.size === 0 && p.deletes.size === 0)) return { edits, deletes };
    const rowById = rowsByKey(tabId);
    for (const [id, edit] of p.edits) {
      const row = rowById.get(id);
      if (row !== undefined) edits.set(row, edit.changes);
    }
    for (const id of p.deletes.keys()) {
      const row = rowById.get(id);
      if (row !== undefined) deletes.add(row);
    }
    return { edits, deletes };
  }

  /** Staged edits/deletes whose row is not on the current page. */
  function offPageCount(tabId: string): number {
    const p = pendingState[tabId];
    if (!p) return 0;
    const on = pendingOnPage(tabId);
    return p.edits.size + p.deletes.size - on.edits.size - on.deletes.size;
  }

  function stageChange(tabId: string, row: number, column: string, value: string | null): void {
    if (committingState[tabId]) return;
    const entry = rowKeyOf(tabId, row);
    if (!entry) return; // no complete primary key on this page: the row is not editable
    const p = ensure(tabId);
    if (p.deletes.has(entry.id)) return; // a row marked for delete is not independently editable
    const existing = p.edits.get(entry.id);
    const original = { ...(existing?.original ?? {}) };
    if (!(column in original)) {
      const col = getPage(tabId)?.columns.findIndex((c) => c.name === column) ?? -1;
      const view = col >= 0 ? cell(tabId, row, col) : null;
      original[column] = view && !view.isNull ? view.text : null;
    }
    p.edits.set(entry.id, {
      key: entry.key,
      changes: { ...(existing?.changes ?? {}), [column]: value },
      original,
    });
  }

  /** One entry per staged (row, column) edit, in staging order, for the preview's Changes view. */
  function editedCells(tabId: string): PendingCellDiff[] {
    const out: PendingCellDiff[] = [];
    const p = pendingState[tabId];
    if (!p) return out;
    for (const [rowId, edit] of p.edits) {
      const keyLabel = Object.keys(edit.key)
        .sort(compareNatural)
        .map((name) => `${name} = ${edit.key[name] ?? nullCellText()}`)
        .join(', ');
      for (const [column, after] of Object.entries(edit.changes)) {
        out.push({
          id: `${rowId}\u0000${column}`,
          keyLabel,
          column,
          before: edit.original[column] ?? null,
          after,
        });
      }
    }
    return out;
  }

  // Every stage/discard function refuses while this tab's commit is in flight: a change staged
  // against ops already on the wire would be dropped by the post-commit clearPending.
  // A plain <input> can't distinguish "clear to NULL" from "clear to empty string" — every inline
  // edit stages the typed text verbatim, `''` included. An explicit NULL affordance is not built;
  // a NULL value's own cell must be retyped, not blanked.
  function stageEdit(tabId: string, row: number, column: string, value: string): void {
    stageChange(tabId, row, column, value);
  }

  // The cell editor's Revert action (CellEditorView.vue's resetBuffer, via SelectedCell.onRevert) —
  // un-stages just this one column's edit rather than the whole row's (discardPending) or an
  // insert's (discardInsertRow). Deleting the row entry outright once its last column reverts, not
  // leaving behind an empty `changes: {}`, is what stops the row from still reading as "edited"
  // (hasPending/the pending-count badge, and DataGrid's own yellow row highlight) after its only
  // edit is undone.
  function discardCellEdit(tabId: string, row: number, column: string): void {
    if (committingState[tabId]) return;
    const entry = rowKeyOf(tabId, row);
    const p = pendingState[tabId];
    const existing = entry ? p?.edits.get(entry.id) : undefined;
    if (!entry || !existing || !(column in existing.changes)) return;
    const { [column]: _discarded, ...rest } = existing.changes;
    const { [column]: _original, ...restOriginal } = existing.original;
    if (Object.keys(rest).length === 0) p?.edits.delete(entry.id);
    else p?.edits.set(entry.id, { key: existing.key, changes: rest, original: restOriginal });
  }

  // The row menu's "Revert row(s)" — un-stages a whole row's pending edit and/or pending delete in
  // one go, sibling to discardCellEdit (one column) and discardPending (the whole tab). A row that
  // was never staged is a silent no-op, so callers can run this over an arbitrary selection without
  // first checking which of those rows actually have something to revert.
  function discardRowChange(tabId: string, row: number): void {
    if (committingState[tabId]) return;
    const entry = rowKeyOf(tabId, row);
    const p = pendingState[tabId];
    if (!entry || !p) return;
    p.edits.delete(entry.id);
    p.deletes.delete(entry.id);
  }

  // D4: the cell menu's "Set NULL" — sibling to stageEdit, skipping the inline <input> (which can
  // only ever produce a string) to stage an actual SQL NULL directly.
  function stageNull(tabId: string, row: number, column: string): void {
    stageChange(tabId, row, column, null);
  }

  // "Duplicate row" — one addInsertRow + stageInsertValue per non-primary-key column, copied
  // from the row's current *effective* value (staged edit if present, else the page's own cell).
  // Primary-key columns are left blank (null, addInsertRow's own default) for the user to fill in
  // — duplicating a PK verbatim would only ever produce a guaranteed-collision insert on commit.
  // A generated column is skipped the same way — the server computes it, so copying its
  // displayed value forward would only ever be rejected on commit.
  function duplicateAsInsert(tabId: string, row: number): string | null {
    if (committingState[tabId]) return null;
    const page = getPage(tabId);
    if (!page) return null;
    const columns = page.columns.filter((c) => !c.isPrimaryKey && !c.generated).map((c) => c.name);
    const id = addInsertRow(tabId, columns);
    for (let col = 0; col < page.columns.length; col++) {
      const descriptor = page.columns[col];
      if (descriptor.isPrimaryKey || descriptor.generated) continue;
      const staged = stagedValue(tabId, row, descriptor.name);
      if (staged !== undefined) {
        if (staged !== null) stageInsertValue(tabId, id, descriptor.name, staged);
        continue;
      }
      const view = cell(tabId, row, col);
      // A truncated cell's `.text` is only the 64 KiB prefix the engine sent, the same reason
      // such a cell is non-editable ("committing the buffer verbatim would write the truncated text
      // over the real value"). Duplicating it staged that prefix as a real insert value, silently
      // corrupting the copy on Commit. Left unset (null, addInsertRow's own default) instead — the
      // same "not carried over" treatment an unresolved value already gets — rather than ever
      // staging a value known to be wrong.
      if (!view.isNull && !view.truncated) stageInsertValue(tabId, id, descriptor.name, view.text);
    }
    return id;
  }

  // Real-interaction fix (reported bug — invoking the delete shortcut a second time on an already-
  // deleted row UNDID the deletion): this used to toggle — an already-pending row was un-marked
  // rather than left alone. Every one of its three callers (the right-click menu's "Delete row(s)",
  // the `grid.deleteRows` keyboard shortcut, and DataToolbar.vue's own toolbar button) presents
  // itself as a plain "delete this" action with no toggle affordance (no distinct icon/label for
  // "already deleted, click to undo"), so a repeat invocation — the same shortcut fired twice by
  // habit, or the same selection re-deleted after extending it — silently reversed the user's own
  // most recent action instead of being a no-op. Marking for delete is now idempotent, exactly like
  // stageEdit/stageNull above never toggle either: "Revert row(s)" (discardRowChange) is the one and
  // only way to undo a pending delete, for every pending-change kind this module has.
  function stageDelete(tabId: string, rows: number[]): void {
    if (committingState[tabId]) return;
    for (const row of rows) {
      const entry = rowKeyOf(tabId, row);
      if (!entry) continue;
      const p = ensure(tabId);
      p.deletes.set(entry.id, { key: entry.key });
      p.edits.delete(entry.id); // a row marked for delete is not independently editable (mirrors stageEdit)
    }
  }

  function addInsertRow(tabId: string, columns: string[]): string {
    if (committingState[tabId]) return '';
    const p = ensure(tabId);
    const id = newUuid();
    const values: Record<string, string | null> = {};
    for (const name of columns) values[name] = null;
    p.inserts.push({ id, values });
    return id;
  }

  function stageInsertValue(tabId: string, insertId: string, column: string, value: string): void {
    if (committingState[tabId]) return;
    const insert = pendingState[tabId]?.inserts.find((i) => i.id === insertId);
    if (insert) insert.values[column] = value;
  }

  function discardInsertRow(tabId: string, insertId: string): void {
    if (committingState[tabId]) return;
    const p = pendingState[tabId];
    if (!p) return;
    p.inserts = p.inserts.filter((i) => i.id !== insertId);
  }

  function buildPlan(tabId: string): MutationRowOp[] | null {
    const p = pendingState[tabId];
    if (!p) return null;
    const ops: MutationRowOp[] = [];
    for (const del of p.deletes.values()) ops.push({ kind: 'delete', key: del.key });
    for (const edit of p.edits.values()) {
      ops.push({ kind: 'update', key: edit.key, changes: edit.changes });
    }
    for (const insert of p.inserts) {
      ops.push({ kind: 'insert', values: insert.values });
    }
    return ops.length > 0 ? ops : null;
  }

  async function previewPending(
    connectionId: string,
    path: string,
    tabId: string,
  ): Promise<string[]> {
    const ops = buildPlan(tabId);
    if (!ops) return [];
    return (await data.preview({ connectionId, path, ops })).statements;
  }

  async function commitPending(
    connectionId: string,
    path: string,
    tabId: string,
    afterCommit?: () => Promise<void>,
  ): Promise<MutateResponse | null> {
    // A second click, or an edit staged mid-flight, would resend or drop ops already on the wire.
    // `afterCommit` runs inside the same window so the reload that replaces the page cannot drop
    // an edit staged against the old one.
    if (committingState[tabId]) return null;
    committingState[tabId] = true;
    try {
      const ops = buildPlan(tabId);
      if (!ops) return null;
      const result = await data.mutate({
        tabId,
        connectionId,
        path,
        ops,
      });
      clearPending(tabId);
      await afterCommit?.();
      return result;
    } finally {
      delete committingState[tabId];
    }
  }

  function discardPending(tabId: string): void {
    if (committingState[tabId]) return;
    clearPending(tabId);
  }

  function isCommitting(tabId: string): boolean {
    return committingState[tabId] === true;
  }

  return {
    pendingFor,
    rawPendingFor,
    hasPending,
    isCommitting,
    clearPending,
    registerFullPrimaryKeyAccessor,
    pageHasFullPrimaryKey,
    isPendingDelete,
    stagedValue,
    stagedOriginal,
    rawStagedValue,
    rawRowChange,
    hasRowChange,
    pendingOnPage,
    offPageCount,
    editedCells,
    stageEdit,
    discardCellEdit,
    discardRowChange,
    stageNull,
    duplicateAsInsert,
    stageDelete,
    addInsertRow,
    stageInsertValue,
    discardInsertRow,
    previewPending,
    commitPending,
    discardPending,
  };
});
