import type { ApprovalItem } from '@shared/domain/approvals';

export interface ApprovalGroup {
  key: string;
  items: ApprovalItem[];
}

/** The group an item belongs to: its agent session, else its ADE task, else its client and folder. */
export function approvalGroupKey(item: ApprovalItem): string {
  const o = item.origin;
  if (o.session) return `session:${o.session}`;
  if (o.task) return `task:${o.task}`;
  return `client:${o.client}|${o.cwd}`;
}

/** Items grouped by requester. Items sort by age inside a group, groups by their oldest item. */
export function groupApprovals(items: readonly ApprovalItem[]): ApprovalGroup[] {
  const byKey = new Map<string, ApprovalItem[]>();
  for (const item of [...items].sort((a, b) => a.createdAt - b.createdAt)) {
    const key = approvalGroupKey(item);
    byKey.set(key, [...(byKey.get(key) ?? []), item]);
  }
  return Array.from(byKey, ([key, group]) => ({ key, items: group }));
}
