import type { Page } from '@shared/protocol/page';
import { applyLoadFailure } from '../viewOp';

// documents/state.ts, grid/state.ts and shared/keyvalue/state.ts each wrote the same
// load() frame around their own `data.read` call — supersede/tab-closed guards, a page-kind check
// thrown as an error, and the applyLoadFailure/onFailure catch tail. What genuinely differs per
// view (the request payload, which runtime fields a successful page populates, and any extra
// per-view reaction) stays with the caller: `apply` runs the caller's own field assignments
// (including its own `rt.status`/`rt.opId` reset — grid's own extra `position.strategy` check and
// `loadMeta` call happen there too, in the same place they always have, after the kind check the
// frame already ran).
//
// Every paged view now goes through it. navigation.ts's own reason for excluding stream from
// `createPageNavigation` (token-only cursor, no `pageIndex`) is unrelated: that applies to page
// navigation, not to this load frame.

interface LoadFrameRuntime {
  status: string;
  opId: string | null;
  error: { code: string; message: string } | null;
  actionError: string | null;
}

export async function runPagedLoad<K extends Page['kind']>(opts: {
  rt: LoadFrameRuntime;
  opId: string;
  /** Whether this id's runtime record is still live — a tab/viewKey can close while the request
   *  above was in flight; `rt` stays a valid reference to the now-detached record regardless. */
  stillMounted(): boolean;
  read(): Promise<{ page: Page }>;
  expectKind: K;
  /** For the "unexpected page kind" error and applyLoadFailure's own tabId param. */
  id: string;
  /** The noun the "unexpected page kind" error names this tab as, e.g. "document tab". */
  tabNoun: string;
  apply(page: Extract<Page, { kind: K }>): void;
  onFailure?(superseded: boolean): void;
}): Promise<void> {
  try {
    const response = await opts.read();
    if (!opts.stillMounted()) return;
    if (opts.rt.opId !== opts.opId) return;
    if (response.page.kind !== opts.expectKind) {
      throw new Error(`unexpected page kind for a ${opts.tabNoun}: ${response.page.kind}`);
    }
    opts.apply(response.page as Extract<Page, { kind: K }>);
  } catch (err) {
    const superseded = opts.rt.opId !== opts.opId;
    applyLoadFailure(opts.rt, opts.opId, err, opts.id);
    opts.onFailure?.(superseded);
  }
}
