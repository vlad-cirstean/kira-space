<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@theme/components/ui/collapsible';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { formatElapsed } from '@theme/format';
import { t } from '@theme/i18n';
import { TONE_TEXT } from '@theme/lib/statusTone';
import { useApprovalsStore } from '@workbench/state/approvals';
import { computed, onErrorCaptured, ref, watch } from 'vue';
import { logWarn } from '../bridge/log';
import ApprovalBox from './ApprovalBox.vue';
import ApprovalDetails from './ApprovalDetails.vue';
import { approvalRequester } from './approvalFooter';
import { type ApprovalRisk, type ApprovalVerdict, useApprovalKind } from './registry';
import { safeKind } from './safeKind';
import { useCountdown } from './useCountdown';

const props = defineProps<{ item: ApprovalItem }>();

const store = useApprovalsStore();
const def = useApprovalKind(props.item.kind);
const remaining = useCountdown(() => props.item.expiresAt || null);
const error = computed(() => store.errors[props.item.id]);

const FALLBACK_TONE = { neutral: 'neutral', caution: 'warn', danger: 'error' } as const;
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
    ? t('common.expired.status')
    : t('workbench.approvals.left.status', { time: formatElapsed(left * 1000) });
});

const verdictSafe = computed(() =>
  safeKind<ApprovalVerdict | null>(props.item.kind, () => def?.verdict?.(props.item) ?? null, null),
);
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
    verdictSafe.value.failed ||
    kindRisks.value.failed,
);
watch(broken, (v) => v && store.markBroken(props.item.id), { immediate: true });

const risks = computed<ApprovalRisk[]>(() => {
  const out: ApprovalRisk[] = [];
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

// The verdict badge row: what the request does and the facts that bound it.
const VERDICT_TONE = { ok: 'ok', warn: 'warn', error: 'error', neutral: 'default' } as const;
const FACT_TONE = { ok: 'ok', warn: 'warn', neutral: 'default', caution: 'chip' } as const;
const verdict = computed<ApprovalVerdict>(
  () =>
    verdictSafe.value.value ?? {
      label: operation.value.value,
      tone: FALLBACK_TONE[props.item.operation.tone],
      icon: 'circle-filled',
    },
);
// One hue per kind from the status palette: 15% fill, full-strength text, 50% edge.
const HUE_CLASS = {
  explain: 'border border-status-blue/50 bg-status-blue/15 text-status-blue',
  read: 'border border-status-green/50 bg-status-green/15 text-status-green',
  write: 'border border-status-amber/50 bg-status-amber/15 text-status-amber',
  schema: 'border border-status-purple/50 bg-status-purple/15 text-status-purple',
  delete: 'border border-status-red/50 bg-status-red/15 text-status-red',
} as const;
// Kira's own read-only check is orange, distinct from the Write amber.
const FACT_CLASS: Record<string, string> = {
  caution: 'border border-status-orange/50 bg-status-orange/15 text-status-orange',
};
const requester = computed(() => approvalRequester(def, props.item).value);
const detailsOpen = ref(false);
</script>

<template>
  <section
    class="flex min-h-0 flex-1 flex-col gap-3 py-3"
    :data-testid="`approval-item-${item.id}`"
    :data-kind="item.kind"
  >
    <div class="flex min-w-0 flex-wrap items-center gap-x-2" data-testid="approval-target">
      <span
        v-if="def?.targetIcon"
        class="inline-flex shrink-0 self-center"
        data-testid="approval-target-icon"
      >
        <component :is="def.targetIcon" :item="item" />
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
    </div>

    <div class="flex flex-wrap items-center gap-x-2 gap-y-1" data-testid="approval-line">
      <Tooltip :disabled="!verdict.tooltip">
        <TooltipTrigger as-child>
          <Badge
            :variant="verdict.hue ? 'chip' : VERDICT_TONE[verdict.tone]"
            class="h-control-lg gap-1.5 px-2 text-kira-md font-medium [&>svg]:size-3.5"
            :class="verdict.hue && HUE_CLASS[verdict.hue]"
            data-testid="approval-operation"
          >
            <CodiconIcon :name="verdict.icon" :size="16" />
            {{ verdict.label }}
            <CodiconIcon
              v-if="verdict.lock"
              name="lock"
              :size="12"
              class="size-3!"
              data-testid="approval-operation-lock"
            />
          </Badge>
        </TooltipTrigger>
        <TooltipContent>{{ verdict.tooltip }}</TooltipContent>
      </Tooltip>
      <Tooltip v-for="(fact, i) in verdict.facts ?? []" :key="`f${i}`" :disabled="!fact.tooltip">
        <TooltipTrigger as-child>
          <Badge
            :variant="FACT_TONE[fact.tone]"
            class="h-control-lg gap-1.5 px-2 text-kira-md"
            :class="FACT_CLASS[fact.tone]"
            data-testid="approval-fact"
          >
            <CodiconIcon :name="fact.icon" :size="13" />
            {{ fact.text }}
          </Badge>
        </TooltipTrigger>
        <TooltipContent>{{ fact.tooltip }}</TooltipContent>
      </Tooltip>
      <span
        v-if="countdown"
        class="ml-auto shrink-0 whitespace-nowrap text-kira-sm text-subtle tabular-nums"
        data-testid="approval-countdown"
      >
        {{ countdown }}
      </span>
    </div>

    <template v-if="def">
      <ApprovalBox v-if="bodyFailed || !def.sections" testid="approval-body">
        <p
          v-if="bodyFailed"
          class="text-kira-md text-muted-foreground"
          data-testid="approval-body-failed"
        >
          {{ t('workbench.approvals.risk.unreadable.tooltip') }}
        </p>
        <component
          :is="def.content"
          v-else
          :item="item"
          @options="(v: unknown) => store.setOption(item.id, v)"
        />
      </ApprovalBox>
      <div v-else class="flex min-w-0 flex-col gap-2" data-testid="approval-body" data-sections>
        <component
          :is="def.content"
          :item="item"
          @options="(v: unknown) => store.setOption(item.id, v)"
        />
      </div>
    </template>

    <ul
      v-if="risks.length"
      class="m-0 flex list-none flex-col gap-1 p-0"
      data-testid="approval-signals"
    >
      <li v-for="(risk, i) in risks" :key="i">
        <Tooltip :disabled="!risk.tooltip">
          <TooltipTrigger as-child>
            <span
              class="inline-flex items-center gap-1.5 text-kira-md"
              :class="TONE_TEXT[risk.tone]"
              :data-testid="risk.testid"
              v-bind="risk.attrs"
            >
              <CodiconIcon :name="RISK_GLYPH[risk.tone]" :size="13" />
              {{ risk.text }}
            </span>
          </TooltipTrigger>
          <TooltipContent>{{ risk.tooltip }}</TooltipContent>
        </Tooltip>
      </li>
    </ul>

    <Collapsible v-if="def" v-model:open="detailsOpen" data-testid="approval-details-section">
      <CollapsibleTrigger as-child>
        <Button
          variant="link-meta"
          size="inline"
          class="gap-1"
          data-testid="approval-details-toggle"
        >
          <CodiconIcon :name="detailsOpen ? 'chevron-down' : 'chevron-right'" :size="12" />
          {{ t('common.details.title') }}
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent class="flex flex-col gap-1 pt-2 pl-4">
        <component :is="def.details" v-if="def.details && !bodyFailed" :item="item" />
        <ApprovalDetails v-if="requester">
          <dt>{{ t('workbench.approvals.askedBy.label') }}</dt>
          <dd data-testid="approval-origin">{{ requester }}</dd>
        </ApprovalDetails>
      </CollapsibleContent>
    </Collapsible>
    <ApprovalDetails v-else-if="requester">
      <dt>{{ t('workbench.approvals.askedBy.label') }}</dt>
      <dd data-testid="approval-origin">{{ requester }}</dd>
    </ApprovalDetails>
  </section>
</template>
