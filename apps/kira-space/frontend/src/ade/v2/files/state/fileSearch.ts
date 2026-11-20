import type { CodeSearchEvent, FileMatches, SearchStats } from '@shared/domain/repo';
import { errorText } from '@workbench/bridge/codedError';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { defineStore } from 'pinia';
import { markRaw } from 'vue';
import { control } from '../../../../bridge/control';
import { createPerRepoState } from '../../../../repo/state/perRepo';

// The task Files tab's content-search store: one entry per task branch worktree, the query and
// options, and the streamed result list. A search runs on Enter or the Search button only, never
// on every keystroke: each run reads every non-skipped file in the worktree. TanStack Query is
// declined here: results arrive as a push stream on kira:code:search routed by searchId, not as a
// request/response.

export interface FileSearchOptions {
  regex: boolean;
  caseSensitive: boolean;
  wholeWord: boolean;
}

function defaultOptions(): FileSearchOptions {
  return { regex: false, caseSensitive: false, wholeWord: false };
}

interface FileSearchState {
  query: string;
  options: FileSearchOptions;
  searchId: string | null; // null when idle
  running: boolean;
  // C14-1: path-sorted (D11), markRaw'd -- see insertByPath's own comment. Never read this
  // directly outside insertByPath/searchRows; go through `filesVersion` for reactivity.
  files: FileMatches[];
  // Bumped once per incoming batch that mutates `files` in place. `files` itself is markRaw'd (so
  // splicing it never walks a reactive proxy -- see insertByPath), which means Vue can no longer
  // see those mutations; this counter is the reactive signal searchRows depends on instead.
  filesVersion: number;
  collapsed: Set<string>;
  stats: SearchStats | null;
  error: string | null;
}

function emptySearchState(): FileSearchState {
  return {
    query: '',
    options: defaultOptions(),
    searchId: null,
    running: false,
    files: markRaw([]),
    filesVersion: 0,
    collapsed: new Set(),
    stats: null,
    error: null,
  };
}

// Workers emit out of order; git ls-files' own output is already sorted, so inserting each
// incoming group at its sorted position gives a stable, alphabetical list that never reshuffles.
// Replaces an existing group for the same path rather than duplicating it, so a repeated path never
// shows twice.
// Pure function over its arguments alone — no reactive state, stays outside the store.
function insertByPath(files: FileMatches[], group: FileMatches): void {
  let lo = 0;
  let hi = files.length;
  while (lo < hi) {
    const mid = (lo + hi) >>> 1;
    if (files[mid].path < group.path) lo = mid + 1;
    else hi = mid;
  }
  if (files[lo]?.path === group.path) files.splice(lo, 1, group);
  else files.splice(lo, 0, group);
}

// The flat row fold for VirtualList (§7.2): one header row per file followed by its own match rows
// unless collapsed — uniform row height, so a 10,000-match run still renders a screenful.
export interface FileSearchRowVm {
  key: string;
  kind: 'file' | 'match';
  path: string;
  // 'file' rows only:
  matchCount?: number;
  collapsed?: boolean;
  fileTruncated?: boolean;
  // 'match' rows only:
  line?: number;
  column?: number;
  endColumn?: number;
  preview?: string;
  previewMatchStart?: number;
  previewMatchEnd?: number;
}

export const useAdeFileSearchStore = defineStore('adeFileSearch', () => {
  const perBranch = createPerRepoState(emptySearchState);
  const byBranch = perBranch.all;
  const { stateFor } = perBranch;

  function searchQuery(branchId: string): string {
    return byBranch.get(branchId)?.query ?? '';
  }

  function setSearchQuery(branchId: string, query: string): void {
    stateFor(branchId).query = query;
  }

  function searchOptions(branchId: string): FileSearchOptions {
    return byBranch.get(branchId)?.options ?? defaultOptions();
  }

  function toggleSearchOption(branchId: string, key: keyof FileSearchOptions): void {
    const state = stateFor(branchId);
    state.options[key] = !state.options[key];
  }

  function searchRunning(branchId: string): boolean {
    return byBranch.get(branchId)?.running ?? false;
  }

  function searchStats(branchId: string): SearchStats | null {
    return byBranch.get(branchId)?.stats ?? null;
  }

  function searchError(branchId: string): string | null {
    return byBranch.get(branchId)?.error ?? null;
  }

  // searchId -> branchId: an incoming event is routed by searchId only (§4.3), so this is how it
  // finds its own state entry without scanning every open repo's own state.
  const branchBySearchId = new Map<string, string>();

  // Created once, lazily, on the first search (D7's own note: a window that never opens a task
  // Files tab should not hold a listener) — never at module load or app boot.
  let unsubscribe: (() => void) | null = null;

  function ensureSubscribed(): void {
    if (unsubscribe) return;
    unsubscribe = control.onCodeSearch(handleCodeSearchEvent);
  }

  // A Wails event can overtake the bound StartSearch call's own reply; hold it until the start
  // resolves (same shape as createOpLogStore's F7 buffer).
  const earlyEvents: CodeSearchEvent[] = [];
  let startsInFlight = 0;

  function handleCodeSearchEvent(event: CodeSearchEvent): void {
    const branchId = branchBySearchId.get(event.searchId);
    if (!branchId) {
      // A superseded (D8) or already-finished search's late batch is dropped.
      if (startsInFlight > 0) earlyEvents.push(event);
      return;
    }
    const state = byBranch.get(branchId);
    if (!state || state.searchId !== event.searchId) return;

    // C13-5: each incoming group (and its own nested matches array) is markRaw'd before it ever
    // touches `state.files` -- once inserted, a group is only ever replaced wholesale
    // (insertByPath's own splice, on a same-path recur), never field-mutated, so there is nothing
    // for Vue's deep reactivity to usefully track inside it.
    //
    // C14-1: `state.files` itself is now ALSO markRaw'd (see emptySearchState/startSearch), so
    // insertByPath's splice runs against a plain array, never a reactive proxy -- C13-5 alone
    // still left every element-shift from a mid-array splice walking Vue's get/set traps, O(F)
    // per insert and O(F^2) over a streaming search. Measured on a capped 10,000-match search:
    // 41,550ms cumulative main-thread cost (one flush as high as 2,100ms) through the reactive
    // array, vs 2ms for the identical splice sequence on a plain one. Since a raw array's
    // in-place mutation notifies no one, `filesVersion` below is what tells searchRows' own
    // computed new rows are ready.
    for (const group of event.files) insertByPath(state.files, markRaw(group));
    if (event.files.length > 0) state.filesVersion++;

    if (event.done) {
      state.running = false;
      state.stats = event.stats ?? null;
      state.error = event.error?.message ?? null;
      branchBySearchId.delete(event.searchId);
    }
  }

  /** Runs branchId's own current query/options — a no-op for a blank query. A rejected call (bad
   *  regex, git unavailable) sets `error` and leaves `running` false, the same "surface it, don't
   *  throw into the caller" shape the Files tab's other loads use. */
  async function startSearch(branchId: string): Promise<void> {
    const state = stateFor(branchId);
    if (state.query.trim() === '') return;
    ensureSubscribed();

    if (state.searchId) branchBySearchId.delete(state.searchId); // supersede (D8) our own previous run.
    state.files = markRaw([]);
    state.filesVersion++;
    state.collapsed = new Set();
    state.stats = null;
    state.error = null;
    state.running = true;
    state.searchId = null;

    startsInFlight++;
    try {
      const { searchId } = await control.adeTaskFilesSearch(branchId, {
        query: state.query,
        regex: state.options.regex,
        caseSensitive: state.options.caseSensitive,
        wholeWord: state.options.wholeWord,
      });
      if (byBranch.get(branchId) !== state) {
        // Released (tab or branch switch) while the start was in flight: the backend reopened the
        // session for it, so stop the search and close that session again. A remount that already
        // owns the branch shares that session: leave it, its next BeginSearch supersedes this one.
        if (!byBranch.has(branchId)) {
          cancelSearch(branchId);
          fireAndForget(control.adeTaskFilesClose(branchId), 'files');
        }
        return;
      }
      state.searchId = searchId;
      branchBySearchId.set(searchId, branchId);
    } catch (err) {
      state.running = false;
      state.error = errorText(err);
    } finally {
      startsInFlight--;
      for (const held of earlyEvents.splice(0)) handleCodeSearchEvent(held);
    }
  }

  /** Stops branchId's own in-flight search, if any — the panel's own Stop button. Fire-and-forget:
   *  the terminal event this run still owes (D7/D8) is what actually clears `running`. */
  function cancelSearch(branchId: string): void {
    fireAndForget(control.adeTaskFilesCancelSearch(branchId), 'search');
  }

  function toggleSearchCollapse(branchId: string, path: string): void {
    const state = stateFor(branchId);
    if (state.collapsed.has(path)) state.collapsed.delete(path);
    else state.collapsed.add(path);
  }

  function searchRows(branchId: string): FileSearchRowVm[] {
    const state = byBranch.get(branchId);
    if (!state) return [];
    // `state.files` is markRaw'd: reading it does not by itself see in-place
    // splice/insert mutations, only reference replacement. `filesVersion` is the reactive
    // dependency this computed needs to re-run as new groups stream in -- read it
    // before touching `files`.
    void state.filesVersion;
    const rows: FileSearchRowVm[] = [];
    for (const group of state.files) {
      const collapsed = state.collapsed.has(group.path);
      rows.push({
        key: `file:${group.path}`,
        kind: 'file',
        path: group.path,
        matchCount: group.matches.length,
        collapsed,
        fileTruncated: group.truncated,
      });
      if (collapsed) continue;
      group.matches.forEach((m, i) => {
        rows.push({
          key: `match:${group.path}:${i}`,
          kind: 'match',
          path: group.path,
          line: m.line,
          column: m.column,
          endColumn: m.endColumn,
          preview: m.preview,
          previewMatchStart: m.previewMatchStart,
          previewMatchEnd: m.previewMatchEnd,
        });
      });
    }
    return rows;
  }

  /** Drops branchId's own cached search state — the Files tab's cleanup when it goes away. */
  function dropSearch(branchId: string): void {
    const state = byBranch.get(branchId);
    if (state?.searchId) branchBySearchId.delete(state.searchId);
    perBranch.drop(branchId);
  }

  return {
    searchQuery,
    setSearchQuery,
    searchOptions,
    toggleSearchOption,
    searchRunning,
    searchStats,
    searchError,
    startSearch,
    cancelSearch,
    toggleSearchCollapse,
    searchRows,
    dropSearch,
  };
});
