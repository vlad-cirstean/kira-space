<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { Button } from '@theme/components/ui/button';
import {
  Tooltip,
  TooltipContent,
  TooltipDisabledTrigger,
  TooltipTrigger,
} from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { useApprovalsStore } from '@workbench/state/approvals';
import { computed, onErrorCaptured, watch } from 'vue';
import { logWarn } from '../bridge/log';
import { approvalLabels, approvalRequester } from './approvalFooter';
import { useApprovalActions } from './approvalsQueries';
import { blankInput } from './blankInput';
import { useApprovalKind } from './registry';

const props = defineProps<{ item: ApprovalItem }>();

const store = useApprovalsStore();
const { answer } = useApprovalActions();
const def = useApprovalKind(props.item.kind);
const busy = computed(() => store.answering.includes(props.item.id));
const labels = computed(() => approvalLabels(def, props.item));
const requester = computed(() => approvalRequester(def, props.item));
// Approve waits for every value the agent asked the person to type.
const blank = computed(() => blankInput(def, props.item, store.inputs[props.item.id]));
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
    <slot name="start" />
    <div class="ml-auto flex shrink-0 items-center gap-3" data-testid="approval-actions">
      <component
        :is="def?.footerAction"
        v-if="def?.footerAction"
        :item="item"
        @options="(v: unknown) => store.setOption(item.id, v)"
      />
      <div class="flex items-center gap-1.5">
        <Button
          variant="dialog-deny"
          size="kira-lg"
          :pending="busy"
          shortcut="prompt.deny"
          :data-testid="`approval-deny-${item.id}`"
          @click="answer([item.id], false)"
        >
          {{ labels.value?.deny ?? t('common.action.deny.label') }}
        </Button>
        <Tooltip v-if="def && !unreadable" :disabled="blank === null">
          <TooltipTrigger as-child>
            <TooltipDisabledTrigger :disabled="blank !== null">
              <Button
                variant="dialog-approve"
                size="kira-lg"
                :pending="busy"
                :disabled="blank !== null"
                shortcut="form.submit"
                :data-testid="`approval-approve-${item.id}`"
                @click="answer([item.id], true)"
              >
                {{ labels.value?.approve ?? t('common.action.approve.label') }}
              </Button>
            </TooltipDisabledTrigger>
          </TooltipTrigger>
          <TooltipContent>{{
            t('workbench.approvals.input.required.tooltip', { label: blank ?? '' })
          }}</TooltipContent>
        </Tooltip>
      </div>
    </div>
  </div>
</template>
