import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem } from '@workbench/testing/ui/approvals';

/** A recorded item given a live clock, its kept variables created a minute ago and expiring in 59. */
export function withTimes(it: ApprovalItem): ApprovalItem {
  const payload = structuredClone(it.payload) as { kept: Record<string, unknown>[] };
  payload.kept = payload.kept.map((k) => ({
    ...k,
    createdAt: new Date(Date.now() - 60_000).toISOString(),
    expiresAt: new Date(Date.now() + 59 * 60_000).toISOString(),
  }));
  return liveItem({ ...it, payload });
}
