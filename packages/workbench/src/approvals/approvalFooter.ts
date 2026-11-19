import type { ApprovalItem } from '@shared/domain/approvals';
import type { ApprovalKindDef } from './registry';
import { type Safe, safeKind } from './safeKind';

// Footer text shared by the desktop footer and the phone list. A throwing kind falls back, so one
// bad payload never blanks the footer; `failed` tells the caller to hide Approve.
export function approvalRequester(
  def: ApprovalKindDef | undefined,
  item: ApprovalItem,
): Safe<string> {
  const o = item.origin;
  const origin = [o.client, o.cwd, o.task, o.step, o.source].filter(Boolean).join(' · ');
  return safeKind(item.kind, () => def?.requester?.(item) ?? origin, origin);
}

export function approvalLabels(
  def: ApprovalKindDef | undefined,
  item: ApprovalItem,
): Safe<{ approve: string; deny: string } | undefined> {
  return safeKind(item.kind, () => def?.labels?.(item), undefined);
}
