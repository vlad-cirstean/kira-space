import type { ApprovalKinds } from '@workbench/approvals/registry';
import { markRaw } from 'vue';
import { branchRequestKind, taskCreateKind } from './kindDefs';
import TaskCreateBody from './TaskCreateBody.vue';

// P520: Space's approval kinds (internal/approvals Kind* constants). The shared surface supplies the
// chrome; one branch.request item holds all of a task's concurrent branch calls; one task.create item is
// one orchestrator request for a child task.
export function spaceApprovalKinds(): ApprovalKinds {
  return {
    'branch.request': branchRequestKind(),
    'task.create': taskCreateKind(markRaw(TaskCreateBody)),
  };
}
