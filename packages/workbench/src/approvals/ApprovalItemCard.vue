<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { FieldDescription } from '@theme/components/ui/field';
import { connColorVar } from '@theme/connColor';
import { t } from '@theme/i18n';
import { useApprovalsStore } from '@workbench/state/approvals';
import { computed } from 'vue';
import { useApprovalActions } from './approvalsQueries';
import { useApprovalKind } from './registry';
import { formatClock, useCountdown } from './useCountdown';

const props = defineProps<{ item: ApprovalItem; first: boolean }>();

const store = useApprovalsStore();
const { answer } = useApprovalActions();
const def = useApprovalKind(props.item.kind);
const remaining = useCountdown(() => props.item.expiresAt || null);
const busy = computed(() => store.answering.includes(props.item.id));
const error = computed(() => store.errors[props.item.id]);
const rail = computed(() => connColorVar(props.item.target.color));
const toneVariant = { neutral: 'default', caution: 'warn', danger: 'error' } as const;

const originLine = computed(() => {
  const o = props.item.origin;
  const origin = [o.client, o.cwd, o.task, o.step, o.source].filter(Boolean).join(' · ');
  return origin ? t('workbench.approvals.origin.status', { origin }) : '';
});
const operation = computed(() => def?.operation?.(props.item) ?? props.item.operation.class);
const labels = computed(() => def?.labels?.(props.item));
</script>

<template>
  <section
    class="flex flex-col gap-2 border-l-[3px] border-border pl-3"
    :style="rail ? { borderLeftColor: rail } : undefined"
    :data-testid="`approval-item-${item.id}`"
    :data-kind="item.kind"
  >
    <div class="flex items-start justify-between gap-2">
      <component :is="def.header" v-if="def?.header" :item="item" />
      <div v-else class="flex min-w-0 flex-col gap-0.5">
        <span
          class="min-w-0 break-words text-kira-lg font-medium"
          data-testid="approval-target-name"
        >
          {{ item.target.name }}
        </span>
        <span class="flex flex-wrap items-baseline gap-x-2 text-kira-sm">
          <span v-if="item.target.kind" data-testid="approval-target-kind">{{
            item.target.kind
          }}</span>
          <span v-if="item.target.location" class="font-data text-muted-foreground">
            {{ item.target.location }}
          </span>
        </span>
      </div>
      <span class="flex shrink-0 items-center gap-1">
        <Badge v-if="item.target.readOnly">{{ t('common.state.readOnly.status') }}</Badge>
        <Badge
          v-if="item.operation.class"
          :variant="toneVariant[item.operation.tone]"
          data-testid="approval-operation"
        >
          {{ operation }}
        </Badge>
        <Badge v-if="remaining !== null" data-testid="approval-countdown">
          {{
            remaining === 0
              ? t('workbench.approvals.expired.status')
              : t('common.expires.status', { time: formatClock(remaining) })
          }}
        </Badge>
      </span>
    </div>

    <FieldDescription v-if="originLine" data-testid="approval-origin">{{
      originLine
    }}</FieldDescription>

    <component
      :is="def.body"
      v-if="def"
      :item="item"
      @options="(v: unknown) => store.setOption(item.id, v)"
    />
    <Alert v-else variant="warn">
      <AlertDescription>{{ t('workbench.approvals.unsupported.body') }}</AlertDescription>
    </Alert>

    <Alert v-if="error" variant="error" data-testid="approval-error">
      <AlertDescription>{{ error }}</AlertDescription>
    </Alert>

    <div class="flex justify-end gap-2">
      <Button
        variant="dialog-danger"
        size="kira-lg"
        :pending="busy"
        :data-dialog-initial="first || undefined"
        :data-testid="`approval-deny-${item.id}`"
        @click="answer([item.id], false)"
      >
        {{ labels?.deny ?? t('common.action.deny.label') }}
      </Button>
      <Button
        v-if="def"
        variant="dialog-primary"
        size="kira-lg"
        :pending="busy"
        :data-testid="`approval-approve-${item.id}`"
        @click="answer([item.id], true)"
      >
        {{ labels?.approve ?? t('common.action.approve.label') }}
      </Button>
    </div>
  </section>
</template>
