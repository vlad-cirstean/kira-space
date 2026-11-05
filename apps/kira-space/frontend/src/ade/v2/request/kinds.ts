import type { ApprovalKinds } from '@workbench/approvals/registry';
import { markRaw } from 'vue';
import BranchRequestBody from './BranchRequestBody.vue';

// P520: Space's approval kinds (internal/approvals Kind* constants). The shared surface supplies the
// chrome; one branch.request item holds all of a task's concurrent branch calls.
export function spaceApprovalKinds(): ApprovalKinds {
  return {
    'branch.request': { body: markRaw(BranchRequestBody) },
  };
}
