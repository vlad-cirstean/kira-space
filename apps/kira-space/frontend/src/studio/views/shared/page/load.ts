import type { Page } from '@shared/protocol/page';
import { applyLoadFailure, beginOp } from '../viewOp';

// The load() frame every paged view's store wraps around its own `data.read` call: op stamp,
// supersede/tab-closed guards, a page-kind check thrown as an error, and the
// applyLoadFailure/onFailure catch tail. What differs per view (the request payload, which runtime
// fields a successful page populates, any extra per-view reaction) stays with the caller:
// `apply` runs the caller's own assignments after the kind check the frame already ran.

interface LoadFrameRuntime {
  status: string;
  opId: string | null;
  error: { code: string; message: string } | null;
  actionError: string | null;
}

export async function runPagedLoad<K extends Page['kind']>(opts: {
  rt: LoadFrameRuntime;
  /** The store's runtime map; the read is dropped once `runtime[id]` is gone (a tab/viewKey can
   *  close while the request is in flight; `rt` stays a valid reference to the detached record). */
  runtime: Record<string, unknown>;
  /** The runtime key; also the "unexpected page kind" error's and applyLoadFailure's tab id. */
  id: string;
  read(opId: string): Promise<{ page: Page }>;
  expectKind: K;
  /** The noun the "unexpected page kind" error names this tab as, e.g. "document tab". */
  tabNoun: string;
  apply(page: Extract<Page, { kind: K }>): void;
  onFailure?(superseded: boolean): void;
}): Promise<boolean> {
  const opId = beginOp(opts.rt);
  try {
    const response = await opts.read(opId);
    if (!opts.runtime[opts.id]) return false;
    if (opts.rt.opId !== opId) return false;
    if (response.page.kind !== opts.expectKind) {
      throw new Error(`unexpected page kind for a ${opts.tabNoun}: ${response.page.kind}`);
    }
    opts.apply(response.page as Extract<Page, { kind: K }>);
    return true;
  } catch (err) {
    const superseded = opts.rt.opId !== opId;
    applyLoadFailure(opts.rt, opId, err, opts.id);
    opts.onFailure?.(superseded);
    return false;
  }
}
