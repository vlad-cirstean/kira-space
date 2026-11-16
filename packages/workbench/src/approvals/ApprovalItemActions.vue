<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { useApprovalsStore } from '@workbench/state/approvals';
import { computed, onErrorCaptured, watch } from 'vue';
import { logWarn } from '../bridge/log';
import { useApprovalActions } from './approvalsQueries';
import { useApprovalKind } from './registry';
import { safeKind } from './safeKind';

const props = defineProps<{ item: ApprovalItem }>();

const store = useApprovalsStore();
const { answer } = useApprovalActions();
const def = useApprovalKind(props.item.kind);
const busy = computed(() => store.answering.includes(props.item.id));
const labels = computed(() =>
  safeKind(props.item.kind, () => def?.labels?.(props.item), undefined),
);
const requester = computed(() => {
  const o = props.item.origin;
  const origin = [o.client, o.cwd, o.task, o.step, o.source].filter(Boolean).join(' · ');
  return safeKind(props.item.kind, () => def?.requester?.(props.item) ?? origin, origin);
});
const unreadable = computed(
  () => store.isBroken(props.item.id) || labels.value.failed || requester.value.failed,
);
// A throwing footer control must not take Deny with it.
onErrorCaptured((err) => {
  logWarn('approvals', `kind ${props.item.kind} unreadable`, err);
  store.markBroken(props.item.id);
  return false;
});
watch(unreadable, (v) => v && store.markBroken(props.item.id), { immediate: true });
</script>

<template>
  <div class="flex w-full items-center gap-3" data-testid="approval-footer">
    <div class="min-w-0 flex-1 text-kira-sm text-subtle" data-testid="approval-origin">
      <TruncatedText :text="requester.value" />
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
        {{ labels.value?.deny ?? t('common.action.deny.label') }}
      </Button>
      <Button
        v-if="def && !unreadable"
        variant="dialog-approve"
        size="kira-lg"
        :pending="busy"
        :data-testid="`approval-approve-${item.id}`"
        @click="answer([item.id], true)"
      >
        {{ labels.value?.approve ?? t('common.action.approve.label') }}
      </Button>
    </div>
  </div>
</template>
