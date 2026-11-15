import { branchRequestKind, detailsEditKind, taskCreateKind } from '@ade/request/kindDefs';
import type { ApprovalKinds } from '@workbench/approvals/registry';
import { markRaw } from 'vue';
import TaskCreateBrief from './TaskCreateBrief.vue';

// The Space kinds the phone answers (mobileweb/approvals.go allowlists the same three).
export function mobileApprovalKinds(): ApprovalKinds {
  return {
    'branch.request': branchRequestKind(),
    'details.edit': detailsEditKind(),
    'task.create': taskCreateKind(markRaw(TaskCreateBrief)),
  };
}
