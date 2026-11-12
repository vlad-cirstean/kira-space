<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { useApprovalsStore } from '@workbench/state/approvals';
import { computed } from 'vue';
import { useApprovalActions } from './approvalsQueries';
import { useApprovalKind } from './registry';

const props = defineProps<{ item: ApprovalItem }>();

const store = useApprovalsStore();
const { answer } = useApprovalActions();
const def = useApprovalKind(props.item.kind);
const busy = computed(() => store.answering.includes(props.item.id));
const labels = computed(() => def?.labels?.(props.item));
const requester = computed(() => {
  if (def?.requester) return def.requester(props.item);
  const o = props.item.origin;
  return [o.client, o.cwd, o.task, o.step, o.source].filter(Boolean).join(' · ');
});
</script>

<template>
  <div class="flex w-full items-center gap-3" data-testid="approval-footer">
    <div class="min-w-0 flex-1 text-kira-sm text-subtle" data-testid="approval-origin">
      <TruncatedText :text="requester" />
    </div>
    <component
      :is="def?.footerAction"
      v-if="def?.footerAction"
      :item="item"
      @options="(v: unknown) => store.setOption(item.id, v)"
    />
    <div class="flex shrink-0 items-center gap-1.5">
      <Button
        variant="dialog-deny"
        size="kira-lg"
        :pending="busy"
        data-dialog-initial
        :data-testid="`approval-deny-${item.id}`"
        @click="answer([item.id], false)"
      >
        {{ labels?.deny ?? t('common.action.deny.label') }}
      </Button>
      <Button
        v-if="def"
        variant="dialog-approve"
        size="kira-lg"
        :pending="busy"
        :data-testid="`approval-approve-${item.id}`"
        @click="answer([item.id], true)"
      >
        {{ labels?.approve ?? t('common.action.approve.label') }}
      </Button>
    </div>
  </div>
</template>
