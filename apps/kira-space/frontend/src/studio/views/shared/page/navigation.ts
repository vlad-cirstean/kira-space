import type { PageCursor } from '@shared/protocol/dataOps';
import { stopOp } from '../viewOp';

// documents/state.ts, grid/state.ts and shared/keyvalue/state.ts each declared the
// same stop/goNext/goPrev/goFirst/goLast/goToPage/resetTokens bodies, differing only in their own
// tab accessor pair (findDocumentTab/patchDocumentTabState, findDataTab/patchDataTabState,
// keyValueHost/host.patch). `host` below is that pair, generalized. stream/state.ts is
// deliberately NOT built on this: its own goNext is token-only with no offset fallback and no
// `pageIndex` to track (StreamTabState carries none, by design — a browse tab has no addressable
// position to go back to), and it has no goPrev/goFirst/goLast/goToPage at all.

export interface PageNavRuntime {
  opId: string | null;
  nextToken: string | null;
  prevToken: string | null;
  count: { value: number } | null;
}

export interface PageNavHost<R extends PageNavRuntime> {
  tab(id: string): { pageIndex: number; pageSize: number } | undefined;
  patch(id: string, patch: { pageIndex: number }): void;
  runtime(id: string): R | undefined;
  ensureRuntime(id: string): R;
  load(id: string, cursor: PageCursor, revertPageIndexOnFailure?: number): Promise<void>;
}

export interface PageNavigation {
  stop(id: string): void;
  goNext(id: string): Promise<void>;
  goPrev(id: string): Promise<void>;
  goFirst(id: string): Promise<void>;
  goLast(id: string): Promise<void>;
  goToPage(id: string, n: number): Promise<void>;
  resetTokens(id: string): void;
}

export function createPageNavigation<R extends PageNavRuntime>(
  host: PageNavHost<R>,
): PageNavigation {
  function stop(id: string): void {
    stopOp(host.runtime(id));
  }

  // The cursor choice: prefer the token when one is available, falling back to offset — the
  // pager position (pageIndex) always advances by one regardless of which strategy served it.
  //
  // A second Next click landing before the first click's read returns used to
  // reuse the *first* click's stale nextToken (the successful load hadn't refreshed it yet) while
  // pageIndex had already been bumped twice — the pager showed page 3 over page 2's
  // requested-but-not-yet-applied rows. rt.opId !== null means a load for this tab is still in
  // flight; ignoring the stray click until it resolves means prevIndex below is always the index
  // of the last page that landed, not a second optimistic guess stacked on the first.
  async function goNext(id: string): Promise<void> {
    const tab = host.tab(id);
    if (!tab) return;
    const rt = host.ensureRuntime(id);
    if (rt.opId !== null) return;
    const prevIndex = tab.pageIndex;
    const nextIndex = prevIndex + 1;
    const cursor: PageCursor = rt.nextToken
      ? { mode: 'after', token: rt.nextToken }
      : { mode: 'offset', offset: nextIndex * tab.pageSize };
    host.patch(id, { pageIndex: nextIndex });
    await host.load(id, cursor, prevIndex);
  }

  // F7 (P108 Part 10): same in-flight guard as goNext, same reason — a stray second click must
  // not stack a second optimistic pageIndex move on top of one whose load hasn't landed yet.
  async function goPrev(id: string): Promise<void> {
    const tab = host.tab(id);
    if (!tab) return;
    const rt = host.ensureRuntime(id);
    if (rt.opId !== null) return;
    const prevIndex = tab.pageIndex;
    const targetIndex = Math.max(0, prevIndex - 1);
    const cursor: PageCursor = rt.prevToken
      ? { mode: 'before', token: rt.prevToken }
      : { mode: 'offset', offset: targetIndex * tab.pageSize };
    host.patch(id, { pageIndex: targetIndex });
    await host.load(id, cursor, prevIndex);
  }

  async function goFirst(id: string): Promise<void> {
    const prevIndex = host.tab(id)?.pageIndex;
    host.patch(id, { pageIndex: 0 });
    await host.load(id, { mode: 'offset', offset: 0 }, prevIndex);
  }

  // Requires a count — the toolbar disables the Last-page button until an exact/estimated count
  // has run.
  async function goLast(id: string): Promise<void> {
    const tab = host.tab(id);
    const rt = host.runtime(id);
    if (!tab || !rt?.count) return;
    const prevIndex = tab.pageIndex;
    const pageCount = Math.max(1, Math.ceil(rt.count.value / tab.pageSize));
    const lastIndex = pageCount - 1;
    host.patch(id, { pageIndex: lastIndex });
    await host.load(id, { mode: 'offset', offset: lastIndex * tab.pageSize }, prevIndex);
  }

  async function goToPage(id: string, n: number): Promise<void> {
    const tab = host.tab(id);
    if (!tab) return;
    const prevIndex = tab.pageIndex;
    const index = Math.max(0, n);
    host.patch(id, { pageIndex: index });
    await host.load(id, { mode: 'offset', offset: index * tab.pageSize }, prevIndex);
  }

  // A keyset token is only meaningful under the query that produced it — every state-changing
  // control resets paging to page 0 and clears whatever tokens were held.
  function resetTokens(id: string): void {
    const rt = host.ensureRuntime(id);
    rt.nextToken = null;
    rt.prevToken = null;
  }

  return { stop, goNext, goPrev, goFirst, goLast, goToPage, resetTokens };
}
