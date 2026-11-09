import type { ApprovalItem } from '@shared/domain/approvals';
import { type Component, type InjectionKey, inject, provide } from 'vue';

export interface ApprovalRisk {
  text: string;
  tone: 'warn' | 'neutral' | 'error';
  testid?: string;
  /** Full detail behind the short tag. */
  tooltip?: string;
  /** Extra attributes on the tag (a `data-*` a spec reads). */
  attrs?: Record<string, string>;
}

/**
 * What an app supplies for one approval kind. The card draws line 1 from `operation`, the target,
 * `where` and `risks`; `content` is the body under it, `details` the label/value rows, `footerAction`
 * a control beside Deny and Approve. Every component takes `{ item }`; `content` and `footerAction`
 * may emit `options`.
 */
export interface ApprovalKindDef {
  content: Component;
  /** The operation word on line 1. */
  operation: (item: ApprovalItem) => string;
  where?: (item: ApprovalItem) => string;
  risks?: (item: ApprovalItem) => ApprovalRisk[];
  /** The footer's left text; the origin (client, folder, task, step, source) when absent. */
  requester?: (item: ApprovalItem) => string;
  details?: Component;
  footerAction?: Component;
  /** The target is data (host, path): data font. */
  targetData?: boolean;
  /** Where is data (folder): data font. */
  whereData?: boolean;
  /** Button words when Approve/Deny do not fit (Send/Withhold, Accept/Reject). */
  labels?: (item: ApprovalItem) => { approve: string; deny: string };
  /** Approve all skips this kind: only a click on the item itself approves it. Deny all still covers it. */
  individualOnly?: boolean;
}

export type ApprovalKinds = Record<string, ApprovalKindDef>;

const approvalKindsKey: InjectionKey<ApprovalKinds> = Symbol('approvalKinds');

export function provideApprovalKinds(kinds: ApprovalKinds): void {
  provide(approvalKindsKey, kinds);
}

export function useApprovalKinds(): ApprovalKinds {
  return inject(approvalKindsKey, {});
}

export function useApprovalKind(kind: string): ApprovalKindDef | undefined {
  return useApprovalKinds()[kind];
}
