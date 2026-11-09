import { t } from '@theme/i18n';
import type { ApprovalKinds } from '@workbench/approvals/registry';
import { markRaw } from 'vue';
import BranchRequestBody from './BranchRequestBody.vue';
import { branchRequestPayloadSchema, taskCreatePayloadSchema } from './payload';
import { branchSource } from './source';
import TaskCreateBody from './TaskCreateBody.vue';
import TaskCreateDetails from './TaskCreateDetails.vue';

// P520: Space's approval kinds (internal/approvals Kind* constants). The shared surface supplies the
// chrome; one branch.request item holds all of a task's concurrent branch calls; one task.create item is
// one orchestrator request for a child task.
export function spaceApprovalKinds(): ApprovalKinds {
  return {
    'branch.request': {
      content: markRaw(BranchRequestBody),
      operation: (item) =>
        t('space.ade.branchRequest.op.label', {
          count: branchRequestPayloadSchema.parse(item.payload).items.length,
        }),
      requester: (item) => {
        const first = branchRequestPayloadSchema.parse(item.payload).items[0];
        return first ? branchSource(first) : '';
      },
    },
    'task.create': {
      content: markRaw(TaskCreateBody),
      details: markRaw(TaskCreateDetails),
      operation: () => t('space.ade.request.taskCreate.op.label'),
      requester: (item) =>
        [taskCreatePayloadSchema.parse(item.payload).parentTitle, item.origin.step]
          .filter(Boolean)
          .join(' · '),
    },
  };
}
