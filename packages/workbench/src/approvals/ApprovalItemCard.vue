<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { colorMarkClass, connBgClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import { useApprovalsStore } from '@workbench/state/approvals';
import { computed, onErrorCaptured, ref, watch } from 'vue';
import { logWarn } from '../bridge/log';
import { type ApprovalRisk, useApprovalKind } from './registry';
import { safeKind } from './safeKind';
import { formatClock, useCountdown } from './useCountdown';

const props = defineProps<{ item: ApprovalItem }>();

const store = useApprovalsStore();
const def = useApprovalKind(props.item.kind);
const remaining = useCountdown(() => props.item.expiresAt || null);
const error = computed(() => store.errors[props.item.id]);

// Tailwind emits only scanned literals: each class is spelled in full.
const TONE_TEXT = {
  neutral: 'text-muted-foreground',
  caution: 'text-warn',
  danger: 'text-error',
} as const;
const RISK_TEXT = {
  warn: 'text-warn-text',
  neutral: 'text-muted-foreground',
  error: 'text-error',
} as const;
const RISK_ICON = {
  warn: 'text-warn',
  neutral: 'text-muted-foreground',
  error: 'text-error',
} as const;
const RISK_GLYPH = { warn: 'warning', neutral: 'info', error: 'error' } as const;

const fallbackOperation = props.item.operation.class;
const operation = computed(() =>
  safeKind(
    props.item.kind,
    () => def?.operation(props.item) ?? fallbackOperation,
    fallbackOperation,
  ),
);
const where = computed(() =>
  safeKind(
    props.item.kind,
    () => {
      if (!def) {
        return [props.item.target.kind, props.item.target.location].filter(Boolean).join(' · ');
      }
      return def.where?.(props.item) ?? '';
    },
    '',
  ),
);
const countdown = computed(() => {
  const left = remaining.value;
  if (left === null) return '';
  return left === 0
    ? t('workbench.approvals.expired.status')
    : t('workbench.approvals.left.status', { time: formatClock(left) });
});

const kindRisks = computed(() =>
  safeKind<ApprovalRisk[]>(props.item.kind, () => def?.risks?.(props.item) ?? [], []),
);
// A body or details component that throws while rendering is dropped for a fallback line.
const bodyFailed = ref(false);
onErrorCaptured((err) => {
  logWarn('approvals', `kind ${props.item.kind} unreadable`, err);
  bodyFailed.value = true;
  return false;
});
const broken = computed(
  () =>
    store.isBroken(props.item.id) ||
    bodyFailed.value ||
    operation.value.failed ||
    where.value.failed ||
    kindRisks.value.failed,
);
watch(broken, (v) => v && store.markBroken(props.item.id), { immediate: true });

const risks = computed<ApprovalRisk[]>(() => {
  const out: ApprovalRisk[] = [];
  if (props.item.target.readOnly) {
    out.push({ text: t('common.state.readOnly.status'), tone: 'neutral' });
  }
  if (def) out.push(...kindRisks.value.value);
  else out.push({ text: t('workbench.approvals.risk.unsupported.status'), tone: 'warn' });
  if (broken.value) {
    out.push({
      text: t('workbench.approvals.risk.unreadable.status'),
      tooltip: t('workbench.approvals.risk.unreadable.tooltip'),
      tone: 'error',
      testid: 'approval-unreadable',
    });
  }
  if (error.value) out.push({ text: error.value, tone: 'error', testid: 'approval-error' });
  return out;
});
</script>

<template>
  <section
    class="flex flex-1 flex-col gap-2 py-3"
    :data-testid="`approval-item-${item.id}`"
    :data-kind="item.kind"
  >
    <div class="flex items-baseline gap-x-3" data-testid="approval-line">
      <div class="flex min-w-0 flex-1 flex-wrap items-baseline gap-x-2 gap-y-0.5">
        <span
          v-if="item.operation.class"
          class="text-kira-lg font-medium"
          :class="TONE_TEXT[item.operation.tone]"
          data-testid="approval-operation"
        >
          {{ operation.value }}
        </span>
        <span
          class="min-w-0 break-words text-kira-lg font-medium text-fg"
          :class="def?.targetData && 'font-data'"
          data-testid="approval-target-name"
        >
          {{ item.target.name }}
        </span>
        <span
          v-if="where.value"
          class="text-kira-md text-muted-foreground"
          :class="def?.whereData && 'font-data'"
          data-testid="approval-target-kind"
        >
          {{ where.value }}
        </span>
        <Tooltip v-for="(risk, i) in risks" :key="i" :disabled="!risk.tooltip">
          <TooltipTrigger as-child>
            <span
              class="inline-flex items-baseline gap-1 text-kira-md"
              :class="[
                RISK_TEXT[risk.tone],
                risk.testid === 'approval-error' || 'whitespace-nowrap',
              ]"
              :data-testid="risk.testid"
              v-bind="risk.attrs"
            >
              <CodiconIcon
                :name="RISK_GLYPH[risk.tone]"
                :size="13"
                class="self-center"
                :class="RISK_ICON[risk.tone]"
              />
              {{ risk.text }}
            </span>
          </TooltipTrigger>
          <TooltipContent>{{ risk.tooltip }}</TooltipContent>
        </Tooltip>
      </div>
      <span
        v-if="countdown"
        class="shrink-0 whitespace-nowrap text-kira-sm text-subtle tabular-nums"
        data-testid="approval-countdown"
      >
        {{ countdown }}
      </span>
    </div>

    <span
      v-if="connBgClass(item.target.color)"
      :class="colorMarkClass('band', item.target.color)"
      data-testid="approval-band"
    />

    <div v-if="def" class="flex min-w-0 flex-1 flex-col gap-2 pl-3" data-testid="approval-body">
      <p
        v-if="bodyFailed"
        class="text-kira-md text-muted-foreground"
        data-testid="approval-body-failed"
      >
        {{ t('workbench.approvals.risk.unreadable.tooltip') }}
      </p>
      <template v-else>
        <component
          :is="def.content"
          :item="item"
          @options="(v: unknown) => store.setOption(item.id, v)"
        />
        <component :is="def.details" v-if="def.details" :item="item" />
      </template>
    </div>
  </section>
</template>
