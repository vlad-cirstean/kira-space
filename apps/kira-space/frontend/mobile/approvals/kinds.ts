import { branchRequestKind, taskCreateKind } from '@ade/request/kindDefs';
import type { ApprovalKinds } from '@workbench/approvals/registry';
import { markRaw } from 'vue';
import TaskCreateBrief from './TaskCreateBrief.vue';

// The Space kinds the phone answers (mobileweb/approvals.go allowlists the same two).
export function mobileApprovalKinds(): ApprovalKinds {
  return {
    'branch.request': branchRequestKind(),
    'task.create': taskCreateKind(markRaw(TaskCreateBrief)),
  };
}
