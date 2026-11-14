import { t } from '@theme/i18n';
import type { ApprovalKindDef } from '@workbench/approvals/registry';
import { type Component, markRaw } from 'vue';
import BranchRequestBody from './BranchRequestBody.vue';
import { branchRequestPayloadSchema, taskCreatePayloadSchema } from './payload';
import { branchSource } from './source';
import TaskCreateDetails from './TaskCreateDetails.vue';

// Space's approval kind defs, shared by the desktop and the phone. The task.create body differs:
// the desktop renders the brief in the notes editor (a bridge-importing chunk the phone build
// cannot load), so each app injects its own `content`.
export function branchRequestKind(): ApprovalKindDef {
  return {
    content: markRaw(BranchRequestBody),
    operation: (item) =>
      t('space.ade.branchRequest.op.label', {
        count: branchRequestPayloadSchema.parse(item.payload).items.length,
      }),
    requester: (item) => {
      const first = branchRequestPayloadSchema.parse(item.payload).items[0];
      return first ? branchSource(first) : '';
    },
  };
}

export function taskCreateKind(content: Component): ApprovalKindDef {
  return {
    content,
    details: markRaw(TaskCreateDetails),
    operation: () => t('space.ade.request.taskCreate.op.label'),
    requester: (item) =>
      [taskCreatePayloadSchema.parse(item.payload).parentTitle, item.origin.step]
        .filter(Boolean)
        .join(' · '),
  };
}
