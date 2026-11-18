<script setup lang="ts">
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { TooltipProvider } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import ApprovalItemCard from '@workbench/approvals/ApprovalItemCard.vue';
import { useApprovalActions, useApprovals } from '@workbench/approvals/approvalsQueries';
import { useApprovalKinds } from '@workbench/approvals/registry';
import { useApprovalsStore } from '@workbench/state/approvals';
import { storeToRefs } from 'pinia';
import { computed, watch } from 'vue';
import { useAuthStore } from '../state/auth';
import PermissionHint from './PermissionHint.vue';

// Pending agent approvals (task.create, branch.request, details.edit) above the board-derived needs. The footer
// is touch sized; answering shares the desktop's store and actions.
const store = useApprovalsStore();
const { answering } = storeToRefs(store);
const { permissions } = storeToRefs(useAuthStore());
const { data } = useApprovals();
const { answer } = useApprovalActions();
const kinds = useApprovalKinds();
const items = computed(() => data.value ?? []);

watch(
  () => items.value.map((i) => i.id),
  (ids) => store.prune(ids),
);

function requester(item: (typeof items.value)[number]): string {
  const def = kinds[item.kind];
  if (def?.requester) return def.requester(item);
  const o = item.origin;
  return [o.client, o.cwd, o.task, o.step, o.source].filter(Boolean).join(' · ');
}
</script>

<template>
  <TooltipProvider>
    <section
      v-if="items.length > 0"
      class="flex flex-col border-b border-border"
      data-testid="phone-approvals"
    >
      <div class="px-1.5 pt-3">
        <SectionHeading :label="t('mobile.approvals.heading.title')">
          <Badge variant="count" data-testid="phone-approvals-count">{{ items.length }}</Badge>
        </SectionHeading>
      </div>
      <PermissionHint v-if="!permissions.agentInput" :what="t('mobile.approvals.answerOff.hint')" />
      <ul class="m-0 flex list-none flex-col p-0">
        <li
          v-for="item in items"
          :key="item.id"
          class="flex flex-col gap-2 border-b border-border px-3 last:border-b-0"
          data-testid="phone-approval"
        >
          <ApprovalItemCard :item="item" />
          <div v-if="permissions.agentInput" class="flex flex-col gap-2 pb-3">
            <span class="min-w-0 truncate text-kira-sm text-subtle" data-testid="approval-origin">
              {{ requester(item) }}
            </span>
            <div class="flex gap-2">
              <Button
                size="touch"
                variant="dialog-deny"
                class="flex-1"
                :pending="answering.includes(item.id)"
                :data-testid="`approval-deny-${item.id}`"
                @click="answer([item.id], false)"
              >
                {{ kinds[item.kind]?.labels?.(item).deny ?? t('common.action.deny.label') }}
              </Button>
              <Button
                v-if="kinds[item.kind]"
                size="touch"
                variant="dialog-approve"
                class="flex-1"
                :pending="answering.includes(item.id)"
                :data-testid="`approval-approve-${item.id}`"
                @click="answer([item.id], true)"
              >
                {{ kinds[item.kind]?.labels?.(item).approve ?? t('common.action.approve.label') }}
              </Button>
            </div>
          </div>
        </li>
      </ul>
    </section>
  </TooltipProvider>
</template>
