import type { MutationRowOp } from '@shared/domain/mutations';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { data } from '../../bridge/data';
import { reloadTabsForTarget } from '../../state/viewCommands';

// The nine-call-site body behind documents/keyvalue/stream's mutations.ts (the ground rules —
// mutate immediately, no pendingChanges.ts-style staged plan, no preview step): resolve the tab,
// one data.mutate, reload the tab's own page, then tell every sibling tab open on the same target.
// `after` covers the one optional tail two of those nine call sites add (deleteKey's
// browseInvalidate) — addKey is not built on this at all, since it opens a new tab instead of
// reloading the current one.
export function createImmediateMutator<
  T extends { connectionId: string | null; path: string },
>(opts: {
  findTab(tabId: string): T | null;
  reload(tabId: string): Promise<void>;
  /** Backstop behind the gated UI: refuses a write whose shown data outlived its connection. */
  isStale(tabId: string): boolean;
}): (
  tabId: string,
  ops: MutationRowOp[],
  after?: (tab: T & { connectionId: string }) => void | Promise<void>,
) => Promise<void> {
  return async (tabId, ops, after) => {
    const tab = opts.findTab(tabId);
    if (!tab?.connectionId) return;
    if (opts.isStale(tabId)) throw new Error(t('studio.common.connectionLost.error'));
    await data.mutate({
      tabId,
      connectionId: tab.connectionId,
      path: tab.path,
      ops,
    });
    // The write above already succeeded — a failure in any step below (this
    // tab's own reload, telling sibling tabs, the caller's `after`) must not read as "the mutation
    // failed" to a caller whose own catch reports it that way; a user retrying on that false
    // signal duplicates a non-idempotent write (Redis list push, stream XADD). Each step gets its
    // own try/catch so one failing never skips the next, and any failures are reported together
    // afterward rather than silently swallowed.
    const failures: string[] = [];
    try {
      await opts.reload(tabId);
    } catch (err) {
      failures.push(errorText(err));
    }
    try {
      reloadTabsForTarget(tab.connectionId, tab.path, tabId);
    } catch (err) {
      failures.push(errorText(err));
    }
    try {
      await after?.(tab as T & { connectionId: string });
    } catch (err) {
      failures.push(errorText(err));
    }
    if (failures.length > 0) {
      throw new Error(t('studio.common.refreshFailed.error', { errors: failures.join('; ') }));
    }
  };
}
