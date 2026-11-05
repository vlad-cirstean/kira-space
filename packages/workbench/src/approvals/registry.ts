import type { ApprovalItem } from '@shared/domain/approvals';
import { type Component, type InjectionKey, inject, provide } from 'vue';

/**
 * What an app supplies for one approval kind. `body` is required; `header` replaces the generic
 * header (target, operation, origin). Every component takes `{ item }`. A body that collects a choice emits `options`.
 */
export interface ApprovalKindDef {
  body: Component;
  header?: Component;
  /** The operation badge text; the raw class when absent. */
  operation?: (item: ApprovalItem) => string;
  /** Button words when Approve/Deny do not fit (Send/Withhold, Accept/Reject). */
  labels?: (item: ApprovalItem) => { approve: string; deny: string };
}

export type ApprovalKinds = Record<string, ApprovalKindDef>;

const approvalKindsKey: InjectionKey<ApprovalKinds> = Symbol('approvalKinds');

export function provideApprovalKinds(kinds: ApprovalKinds): void {
  provide(approvalKindsKey, kinds);
}

export function useApprovalKind(kind: string): ApprovalKindDef | undefined {
  return inject(approvalKindsKey, {})[kind];
}
